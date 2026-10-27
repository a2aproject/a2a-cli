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
	"fmt"
	"net/url"
	"slices"

	"github.com/a2aproject/a2a-cli/internal/credhelper"
)

// targetURL returns the HTTP(S) URL of the agent the command talks to, or
// "" when there is none, such as for a local card file.
func (g *globalConfig) targetURL() string {
	target := g.url
	if target == "" && g.agentCard.IsSet() {
		target = g.agentCard.URL()
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return target
}

// credentialHelper returns the --credential-helper for target.
func (g *globalConfig) credentialHelper(target string) (credhelper.Helper, bool, error) {
	helpers, err := credhelper.ParseAll(g.credentialHelpers)
	if err != nil {
		return credhelper.Helper{}, false, err
	}
	h, ok := credhelper.Select(helpers, target)
	return h, ok, nil
}

// applyCredentialHelper sends the headers the credential helper returns for
// the target agent, unless --auth or --svc-param set the same header.
func applyCredentialHelper(ctx context.Context, cfg *globalConfig) error {
	target := cfg.targetURL()
	if target == "" || len(cfg.credentialHelpers) == 0 {
		return nil
	}
	h, ok, err := cfg.credentialHelper(target)
	if err != nil || !ok {
		return err
	}
	cfg.logf("asking credential helper %q for %s", h.Spec, target)
	resp, err := h.Get(ctx, target)
	if err != nil {
		return fmt.Errorf("getting credentials for %s: %w", target, err)
	}
	names := make([]string, 0, len(resp.Headers))
	for name := range resp.Headers {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		cfg.svcParams.SetDefault(name, resp.Headers[name]...)
	}
	return nil
}
