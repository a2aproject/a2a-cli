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

package agentsrv

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/controlapi"
)

// fakeBotAPI accepts the bot token "good" and rejects every other one.
func fakeBotAPI(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := `{"ok":true,"result":true}`
		switch {
		case !strings.HasPrefix(r.URL.Path, "/botgood/"):
			w.WriteHeader(http.StatusUnauthorized)
			body = `{"ok":false,"error_code":401,"description":"Unauthorized"}`
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			body = `{"ok":true,"result":{"id":99,"is_bot":true,"first_name":"Coder","username":"my_coder_bot"}}`
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			time.Sleep(100 * time.Millisecond)
			body = `{"ok":true,"result":[]}`
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("writing %s: %v", r.URL.Path, err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestAddChannelSavesOnlyValidTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		token       string
		wantErr     bool
		wantChannel *agentdir.TelegramConfig
		wantSecrets agentdir.Secrets
	}{
		{name: "invalid token", token: "bad", wantErr: true, wantSecrets: agentdir.Secrets{}},
		{
			name:        "valid token",
			token:       "good",
			wantChannel: &agentdir.TelegramConfig{Token: "secret:" + agentdir.SecretTelegram},
			wantSecrets: agentdir.Secrets{agentdir.SecretTelegram: "good"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts := startServer(t, serverOpts{name: "assistant", telegramAPI: fakeBotAPI(t)})
			_, err := controlapi.NewClient(ts.dir).AddChannel(context.Background(), controlapi.ChannelRequest{
				Kind:    agentdir.ChannelTelegram,
				Token:   "secret:" + agentdir.SecretTelegram,
				Secrets: map[string]string{agentdir.SecretTelegram: tc.token},
			})
			if (err != nil) != tc.wantErr {
				t.Fatalf("AddChannel() error = %v, want error %v", err, tc.wantErr)
			}
			conf, err := ts.dir.LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			if diff := cmp.Diff(tc.wantChannel, conf.Channels.Telegram); diff != "" {
				t.Fatalf("LoadConfig() wrong telegram channel (-want +got) diff = %s", diff)
			}
			secrets, err := ts.dir.LoadSecrets()
			if err != nil {
				t.Fatalf("LoadSecrets() error = %v", err)
			}
			if diff := cmp.Diff(tc.wantSecrets, secrets); diff != "" {
				t.Fatalf("LoadSecrets() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}
