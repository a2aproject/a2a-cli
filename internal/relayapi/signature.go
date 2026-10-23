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

package relayapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// SignatureHeader carries the HMAC-SHA256 of a wake: t=<unix time>,v1=<hex of HMAC(secret, "<t>.<body>")>.
const SignatureHeader = "X-A2A-Serve-Signature"

// WakeHeader carries the ID of a wake, the same for every retry of it.
const WakeHeader = "X-A2A-Serve-Relay-Wake"

// Sign returns the signature header value of a wake body.
func Sign(secret string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac(secret, ts, body))
}

// ErrBadSignature is returned by [Verify] for a wake that wasn't signed with
// the node's secret or was signed too long ago.
var ErrBadSignature = errors.New("invalid wake signature")

// Verify checks the signature header of a wake body. A signature older or
// newer than maxSkew is rejected; zero accepts any time.
func Verify(secret, header string, body []byte, now time.Time, maxSkew time.Duration) error {
	tPart, vPart, ok := strings.Cut(header, ",")
	ts, okT := strings.CutPrefix(tPart, "t=")
	sig, okV := strings.CutPrefix(vPart, "v1=")
	if !ok || !okT || !okV {
		return ErrBadSignature
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	if maxSkew > 0 && now.Sub(time.Unix(unix, 0)).Abs() > maxSkew {
		return ErrBadSignature
	}
	got, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(got, mac(secret, ts, body)) {
		return ErrBadSignature
	}
	return nil
}

func mac(secret, ts string, body []byte) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "."))
	m.Write(body)
	return m.Sum(nil)
}
