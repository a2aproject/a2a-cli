// Copyright 2026 The A2A Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package email is the email channel of an agent server: IMAP IDLE on a
// dedicated mailbox and SMTP for replies, so no MX record or public URL is
// needed. Only mail which passed DMARC or aligned DKIM at the trusted
// receiving server can instruct the agent or answer approvals.
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/smtp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	_ "github.com/emersion/go-message/charset" // decodes non-UTF-8 mail
	"github.com/emersion/go-message/mail"
	"golang.org/x/net/publicsuffix"

	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// Config configures the mailbox of the agent.
type Config struct {
	Address string
	// IMAP and SMTP are host:port. Ports 993 and 465 use implicit TLS, others STARTTLS.
	IMAP     string
	SMTP     string
	Username string
	Password string
	Mailbox  string
	// TrustedAuthServ is the authserv-id of the receiving server whose
	// Authentication-Results headers are trusted.
	TrustedAuthServ string
}

const (
	maxText     = 100_000
	idleLimit   = 25 * time.Minute
	smtpTimeout = 2 * time.Minute
)

// Channel is a mailbox.
type Channel struct {
	cfg Config
	kv  channel.KV

	mu sync.Mutex
}

var (
	_ channel.Channel   = (*Channel)(nil)
	_ channel.Describer = (*Channel)(nil)
)

// New returns a channel for the mailbox. kv keeps the last UID processed.
func New(cfg Config, kv channel.KV) *Channel {
	if cfg.Mailbox == "" {
		cfg.Mailbox = "INBOX"
	}
	if cfg.Username == "" {
		cfg.Username = cfg.Address
	}
	return &Channel{cfg: cfg, kv: kv}
}

// Name implements [channel.Channel].
func (c *Channel) Name() string { return "email" }

// Caps implements [channel.Channel]. Email sends questions and the final
// result, not a stream.
func (c *Channel) Caps() channel.Caps { return channel.Caps{MaxText: maxText} }

// Describe implements [channel.Describer].
func (c *Channel) Describe() string { return c.cfg.Address }

// LinkHint implements [channel.Describer].
func (c *Channel) LinkHint(code string) string {
	return fmt.Sprintf("email %s with the text: link %s", c.cfg.Address, code)
}

// Run implements [channel.Channel]. It reconnects with backoff.
func (c *Channel) Run(ctx context.Context, in chan<- channel.Inbound) error {
	if c.cfg.TrustedAuthServ == "" {
		return errors.New("email: trusted_authserv must be set, or no mail could be trusted")
	}
	backoff := time.Second
	for ctx.Err() == nil {
		err := c.session(ctx, in)
		if ctx.Err() != nil {
			return nil
		}
		slog.Warn("email: IMAP session ended, reconnecting", "error", err, "in", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 5*time.Minute {
			backoff *= 2
		}
	}
	return nil
}

func dialIMAP(addr string, opts *imapclient.Options) (*imapclient.Client, error) {
	if strings.HasSuffix(addr, ":993") {
		return imapclient.DialTLS(addr, opts)
	}
	return imapclient.DialStartTLS(addr, opts)
}

// session processes new mail, then idles until more arrives.
func (c *Channel) session(ctx context.Context, in chan<- channel.Inbound) error {
	wake := make(chan struct{}, 1)
	client, err := dialIMAP(c.cfg.IMAP, &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data.NumMessages != nil {
					select {
					case wake <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Debug("email: closing IMAP", "error", err)
		}
	}()
	if err := client.Login(c.cfg.Username, c.cfg.Password).Wait(); err != nil {
		return fmt.Errorf("IMAP login: %w", err)
	}
	sel, err := client.Select(c.cfg.Mailbox, nil).Wait()
	if err != nil {
		return fmt.Errorf("selecting %s: %w", c.cfg.Mailbox, err)
	}
	if err := c.checkValidity(ctx, sel); err != nil {
		return err
	}
	for {
		if err := c.fetchNew(ctx, client, in); err != nil {
			return err
		}
		idle, err := client.Idle()
		if err != nil {
			return fmt.Errorf("IMAP IDLE: %w", err)
		}
		select {
		case <-ctx.Done():
		case <-wake:
		case <-time.After(idleLimit):
		}
		if err := idle.Close(); err != nil {
			return err
		}
		if err := idle.Wait(); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

const (
	uidValidityKey = "email:uidvalidity"
	lastUIDKey     = "email:lastuid"
)

// checkValidity starts from the newest message when the mailbox is new to
// the agent or was recreated, instead of answering old mail.
func (c *Channel) checkValidity(ctx context.Context, sel *imap.SelectData) error {
	v := strconv.FormatUint(uint64(sel.UIDValidity), 10)
	cur, ok, err := c.kv.Get(ctx, uidValidityKey)
	if err != nil {
		return err
	}
	if ok && cur == v {
		return nil
	}
	last := uint32(0)
	if sel.UIDNext > 0 {
		last = uint32(sel.UIDNext) - 1
	}
	if err := c.kv.Set(ctx, lastUIDKey, strconv.FormatUint(uint64(last), 10)); err != nil {
		return err
	}
	return c.kv.Set(ctx, uidValidityKey, v)
}

func (c *Channel) fetchNew(ctx context.Context, client *imapclient.Client, in chan<- channel.Inbound) error {
	lastStr, ok, err := c.kv.Get(ctx, lastUIDKey)
	if err != nil {
		return err
	}
	var last uint64
	if ok {
		if last, err = strconv.ParseUint(lastStr, 10, 32); err != nil {
			return fmt.Errorf("email: invalid last UID %q: %w", lastStr, err)
		}
	}
	var set imap.UIDSet
	set.AddRange(imap.UID(last+1), 0)
	section := &imap.FetchItemBodySection{Peek: true}
	msgs, err := client.Fetch(set, &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
	if err != nil {
		return fmt.Errorf("IMAP fetch: %w", err)
	}
	for _, m := range msgs {
		if uint64(m.UID) <= last {
			continue
		}
		raw := m.FindBodySection(section)
		msg, ok, err := c.parse(raw)
		switch {
		case err != nil:
			slog.Warn("email: parsing message", "uid", m.UID, "error", err)
		case ok:
			select {
			case in <- msg:
			case <-ctx.Done():
				return nil
			}
		}
		last = uint64(m.UID)
		if err := c.kv.Set(ctx, lastUIDKey, strconv.FormatUint(last, 10)); err != nil {
			return err
		}
	}
	return nil
}

// parse turns a raw message into an inbound message. It reports false for
// mail the agent must not answer: its own, auto-replies and lists.
func (c *Channel) parse(raw []byte) (channel.Inbound, bool, error) {
	r, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return channel.Inbound{}, false, err
	}
	h := r.Header
	from, err := h.AddressList("From")
	if err != nil || len(from) == 0 {
		return channel.Inbound{}, false, fmt.Errorf("no From address: %w", err)
	}
	// DMARC checks one of several From domains, so the sender would be ambiguous.
	if len(from) > 1 {
		return channel.Inbound{}, false, nil
	}
	sender := strings.ToLower(from[0].Address)
	if strings.EqualFold(sender, c.cfg.Address) {
		return channel.Inbound{}, false, nil
	}
	if auto := strings.ToLower(h.Get("Auto-Submitted")); auto != "" && auto != "no" {
		return channel.Inbound{}, false, nil
	}
	switch strings.ToLower(h.Get("Precedence")) {
	case "bulk", "list", "junk", "auto_reply":
		return channel.Inbound{}, false, nil
	}
	if h.Get("List-Id") != "" || h.Get("List-Unsubscribe") != "" {
		return channel.Inbound{}, false, nil
	}
	msgID, err := h.MessageID()
	if err != nil || msgID == "" {
		return channel.Inbound{}, false, fmt.Errorf("no Message-ID: %w", err)
	}
	root := msgID
	if refs, err := h.MsgIDList("References"); err == nil && len(refs) > 0 {
		root = refs[0]
	} else if irt, err := h.MsgIDList("In-Reply-To"); err == nil && len(irt) > 0 {
		root = irt[0]
	}
	subject, err := h.Subject()
	if err != nil {
		slog.Debug("email: undecodable subject", "error", err)
		subject = ""
	}

	in := channel.Inbound{
		Channel:   c.Name(),
		MessageID: msgID,
		Identity:  "mail:" + sender,
		Name:      from[0].Name,
		Conv:      ConvKey(sender, root, subject),
		Direct:    true,
		Authentic: Authentic(h.Values("Authentication-Results"), c.cfg.TrustedAuthServ, domainOf(sender)),
	}
	var text string
	for {
		p, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return channel.Inbound{}, false, err
		}
		body, err := io.ReadAll(io.LimitReader(p.Body, 20<<20))
		if err != nil {
			return channel.Inbound{}, false, err
		}
		switch ph := p.Header.(type) {
		case *mail.InlineHeader:
			ct, _, err := ph.ContentType()
			if err != nil {
				slog.Debug("email: inline part without a valid content type", "error", err)
				continue
			}
			if ct == "text/plain" && text == "" {
				text = string(body)
			}
		case *mail.AttachmentHeader:
			name, err := ph.Filename()
			if err != nil {
				slog.Debug("email: attachment without a valid filename", "error", err)
			}
			ct, _, err := ph.ContentType()
			if err != nil {
				slog.Debug("email: attachment without a valid content type", "error", err)
				ct = "application/octet-stream"
			}
			in.Files = append(in.Files, channel.File{Name: name, MediaType: ct, Data: body})
		}
	}
	in.Text = StripQuoted(text)
	return in, true, nil
}

// ConvKey identifies an email thread: the correspondent, the thread root and
// the subject, separated by tabs.
func ConvKey(addr, root, subject string) channel.ConvKey {
	return channel.ConvKey(addr + "\t" + root + "\t" + strings.ReplaceAll(subject, "\t", " "))
}

func parseConv(conv channel.ConvKey) (addr, root, subject string) {
	parts := strings.SplitN(string(conv), "\t", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	return parts[0], parts[1], parts[2]
}

func domainOf(addr string) string {
	_, d, _ := strings.Cut(addr, "@")
	return strings.ToLower(d)
}

// Authentic reports whether the topmost Authentication-Results header added
// by the trusted server shows dmarc=pass for the sender's domain, or dkim=pass
// aligned with it. Headers further down may have been forged by the sender.
func Authentic(results []string, trusted, fromDomain string) bool {
	for _, r := range results {
		id, rest, ok := strings.Cut(r, ";")
		if !ok || !strings.EqualFold(strings.Fields(id + " x")[0], trusted) {
			continue
		}
		for res := range strings.SplitSeq(rest, ";") {
			fields := strings.Fields(strings.ToLower(res))
			if len(fields) == 0 {
				continue
			}
			switch fields[0] {
			case "dmarc=pass":
				if slices.Contains(fields[1:], "header.from="+fromDomain) {
					return true
				}
			case "dkim=pass":
				for _, f := range fields[1:] {
					if d, ok := strings.CutPrefix(f, "header.d="); ok && aligned(d, fromDomain) {
						return true
					}
				}
			}
		}
		return false
	}
	return false
}

// aligned reports DMARC relaxed alignment: both domains share their
// organizational domain, such as mail.alice.org and alice.org.
func aligned(d, fromDomain string) bool {
	a, err := publicsuffix.EffectiveTLDPlusOne(d)
	if err != nil {
		return false
	}
	b, err := publicsuffix.EffectiveTLDPlusOne(fromDomain)
	return err == nil && a == b
}

// StripQuoted drops the quoted text of a reply.
func StripQuoted(text string) string {
	var out []string
	for line := range strings.SplitSeq(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ">") {
			continue
		}
		if strings.HasPrefix(trimmed, "On ") && strings.HasSuffix(trimmed, "wrote:") {
			break
		}
		if trimmed == "-- " || trimmed == "--" {
			break
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Send implements [channel.Channel].
func (c *Channel) Send(ctx context.Context, m channel.Outbound) (channel.Ref, error) {
	to, root, subject := parseConv(m.Conv)
	if to == "" {
		return channel.Ref{}, fmt.Errorf("email: no recipient in conversation %q", m.Conv)
	}
	if subject == "" {
		subject = m.Subject
	}
	if root != "" && !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	msgID := store.RandomString(24, store.Alphanumeric) + "@" + domainOf(c.cfg.Address)
	h := mailHeader(c.cfg.Address, to)
	h.SetSubject(subject)
	h.SetMessageID(msgID)
	h.Set("Auto-Submitted", "auto-replied")
	if root != "" {
		h.SetMsgIDList("In-Reply-To", []string{root})
		h.SetMsgIDList("References", []string{root})
	}
	var buf bytes.Buffer
	if err := writeMessage(&buf, h, m); err != nil {
		return channel.Ref{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.sendSMTP(ctx, to, buf.Bytes()); err != nil {
		return channel.Ref{}, err
	}
	return channel.Ref{Conv: m.Conv, ID: msgID}, nil
}

func mailHeader(from, to string) mail.Header {
	var h mail.Header
	h.SetDate(time.Now())
	h.SetAddressList("From", []*mail.Address{{Address: from}})
	h.SetAddressList("To", []*mail.Address{{Address: to}})
	return h
}

func writeMessage(w io.Writer, h mail.Header, m channel.Outbound) error {
	mw, err := mail.CreateWriter(w, h)
	if err != nil {
		return err
	}
	tw, err := mw.CreateInline()
	if err != nil {
		return err
	}
	var th mail.InlineHeader
	th.Set("Content-Type", "text/plain; charset=utf-8")
	pw, err := tw.CreatePart(th)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(pw, m.Text); err != nil {
		return err
	}
	if err := errors.Join(pw.Close(), tw.Close()); err != nil {
		return err
	}
	for _, f := range m.Files {
		var ah mail.AttachmentHeader
		mt := f.MediaType
		if mt == "" {
			mt = "application/octet-stream"
		}
		ah.Set("Content-Type", mt)
		ah.SetFilename(f.Name)
		aw, err := mw.CreateAttachment(ah)
		if err != nil {
			return err
		}
		if _, err := aw.Write(f.Data); err != nil {
			return err
		}
		if err := aw.Close(); err != nil {
			return err
		}
	}
	return mw.Close()
}

func (c *Channel) sendSMTP(ctx context.Context, to string, msg []byte) error {
	host, _, err := net.SplitHostPort(c.cfg.SMTP)
	if err != nil {
		return fmt.Errorf("email: smtp must be host:port: %w", err)
	}
	d := net.Dialer{Timeout: 30 * time.Second}
	var conn net.Conn
	if strings.HasSuffix(c.cfg.SMTP, ":465") {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: host}}).DialContext(ctx, "tcp", c.cfg.SMTP)
	} else {
		conn, err = d.DialContext(ctx, "tcp", c.cfg.SMTP)
	}
	if err != nil {
		return fmt.Errorf("email: SMTP: %w", err)
	}
	// A stalled server would otherwise block every later reply, which wait
	// for c.mu.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(smtpTimeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return errors.Join(fmt.Errorf("email: SMTP: %w", err), conn.Close())
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return errors.Join(fmt.Errorf("email: SMTP: %w", err), conn.Close())
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Debug("email: closing SMTP", "error", err)
		}
	}()
	_, implicitTLS := conn.(*tls.Conn)
	switch ok, _ := client.Extension("STARTTLS"); {
	case ok:
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("email: STARTTLS: %w", err)
		}
	case !implicitTLS && !isLoopback(host):
		return fmt.Errorf("email: %s doesn't offer STARTTLS; refusing to send the password unencrypted", c.cfg.SMTP)
	}
	if err := client.Auth(smtp.PlainAuth("", c.cfg.Username, c.cfg.Password, host)); err != nil {
		return fmt.Errorf("email: SMTP auth: %w", err)
	}
	if err := client.Mail(c.cfg.Address); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return errors.Join(err, w.Close())
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Edit implements [channel.Channel]. Email can't edit sent messages.
func (c *Channel) Edit(context.Context, channel.Ref, channel.Outbound) error {
	return errors.New("email messages can't be edited")
}
