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

// Package pairapi is the pairing endpoint of an agent server, POST
// <agent URL>/pair, which exchanges a one-time code for a token.
package pairapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-cli/internal/jsonhttp"
)

// Path is the path of the pairing endpoint under the agent URL.
const Path = "/pair"

// Request is the body of a pairing request.
type Request struct {
	Code string `json:"code"`
}

// Kinds of tokens a pairing returns.
const (
	KindCaller = "caller"
	KindNode   = "node"
)

// Response is returned for a valid code. The token is shown once. URL is
// where the token applies: the agent for callers, the node for nodes.
type Response struct {
	Kind      string `json:"kind"`
	Token     string `json:"token"`
	Principal string `json:"principal"`
	Agent     string `json:"agent"`
	URL       string `json:"url"`
}

// Pair exchanges code at the agent at agentURL for a token.
func Pair(ctx context.Context, hc *http.Client, agentURL, code string) (Response, error) {
	agentURL = strings.TrimSuffix(agentURL, "/")
	body, err := json.Marshal(Request{Code: code})
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentURL+Path, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return Response{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := errors.Join(readErr, resp.Body.Close()); err != nil {
		return Response{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("pairing with %s: %s", agentURL, jsonhttp.ErrorMessage(resp.Status, data))
	}
	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return Response{}, fmt.Errorf("pairing with %s: %w", agentURL, err)
	}
	return out, nil
}
