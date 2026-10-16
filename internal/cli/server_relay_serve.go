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

package cli

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/relay"
)

func newServerRelayServeCmd(cfg *globalConfig) *cobra.Command {
	var (
		addr        string
		publicURL   string
		adminToken  string
		stateDir    string
		taskTimeout time.Duration
		pairingTTL  time.Duration
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run a relay",
		Long: `Run a relay. Attached nodes are kept in --state-dir; tasks and inboxes are in
memory, so run a single instance that stays up. See deploy/relay for a Cloud
Run deployment.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signalContext(cmd.Context())
			defer stop()
			if cfg.verbose {
				slog.SetLogLoggerLevel(slog.LevelDebug)
			}
			token := relay.Secret(adminToken)
			if token == "" {
				token = rand.Text()
				slog.Warn("no admin token configured, generated one for this run", "admin_token", token)
			}
			srv, err := relay.New(relay.Config{
				PublicURL: publicURL, AdminToken: token, StateDir: stateDir,
				TaskTimeout: taskTimeout, PairingTTL: pairingTTL,
			})
			if err != nil {
				return err
			}
			l, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := hs.Shutdown(shutdownCtx); err != nil {
					slog.Warn("relay shutdown", "error", err)
				}
			}()
			slog.Info("relay listening", "addr", l.Addr().String(), "public_url", publicURL, "state_dir", stateDir)
			if err := hs.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
	defaultAddr := "127.0.0.1:8090"
	if p := os.Getenv("PORT"); p != "" {
		defaultAddr = ":" + p
	}
	defaultStateDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		defaultStateDir = filepath.Join(home, ".local", "state", "a2a-cli", "relay")
	}
	if v, ok := os.LookupEnv("A2A_RELAY_STATE_DIR"); ok {
		defaultStateDir = v
	}
	f := cmd.Flags()
	f.StringVar(&addr, "addr", defaultAddr, "Address to listen on ($PORT sets the default to :$PORT)")
	f.StringVar(&publicURL, "public-url", os.Getenv("A2A_RELAY_PUBLIC_URL"), "Public base URL of the relay (default: derived from request headers)")
	f.StringVar(&adminToken, "admin-token", "env:"+relay.EnvAdminToken, "Admin token, or env:VAR; generated and printed if empty")
	f.StringVar(&stateDir, "state-dir", defaultStateDir, "Where attached nodes are kept ($A2A_RELAY_STATE_DIR); empty keeps them in memory")
	f.DurationVar(&taskTimeout, "task-timeout", time.Hour, "Fail a task when its node posts nothing for this long")
	f.DurationVar(&pairingTTL, "pairing-ttl", 15*time.Minute, "How long a pairing code is valid")
	return cmd
}
