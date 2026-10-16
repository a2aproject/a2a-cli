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

package localsrv

import (
	"context"

	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// ServeAgent runs a local server for an arbitrary executor, such as an agent
// harness. The card advertises streaming.
func ServeAgent(ctx context.Context, cfg Config, executor a2asrv.AgentExecutor) error {
	card, err := createAgentCard(cfg.CardParams)
	if err != nil {
		return err
	}
	handler := a2asrv.NewHandler(executor, a2asrv.WithCapabilityChecks(&card.Capabilities))
	if !cfg.Quiet {
		cfg.Logger("agent %q, transport=%s protocol=%s", card.Name, cfg.Transport, cfg.ProtocolVersion)
	}
	return serve(ctx, cfg, handler, card)
}
