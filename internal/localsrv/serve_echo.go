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
	"iter"

	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

func ServeEcho(ctx context.Context, cfg Config) error {
	if cfg.AgentName == "" {
		cfg.AgentName = "Echo Agent"
	}
	if cfg.AgentDesc == "" {
		cfg.AgentDesc = "Echoes the user's message back as a response"
	}

	card, err := createAgentCard(cfg.CardParams)
	if err != nil {
		return err
	}

	handler := a2asrv.NewHandler(&echoExecutor{})

	if !cfg.Quiet {
		cfg.Logger("echo mode, transport=%s protocol=%s", cfg.Transport, cfg.ProtocolVersion)
	}

	return serve(ctx, cfg, handler, card)
}

type echoExecutor struct{}

func NewEchoExecutor() a2asrv.AgentExecutor {
	return &echoExecutor{}
}

func (e *echoExecutor) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}
		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}
		text := output.MessageText(execCtx.Message)
		evt := a2a.NewArtifactEvent(execCtx, a2a.NewTextPart(text))
		evt.LastChunk = true
		if !yield(evt, nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
	}
}

func (e *echoExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}
