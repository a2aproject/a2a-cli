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

package flagparse

import (
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Transports parses transport aliases (rest, jsonrpc, grpc) into their
// corresponding a2a.TransportProtocol values, preserving order.
func Transports(ss []string) ([]a2a.TransportProtocol, error) {
	if len(ss) == 0 {
		return nil, nil
	}
	protos := make([]a2a.TransportProtocol, 0, len(ss))
	for _, s := range ss {
		proto, err := parseTransport(s)
		if err != nil {
			return nil, err
		}
		protos = append(protos, proto)
	}
	return protos, nil
}

// SingleTransport parses exactly one transport alias, returning an error when
// the input does not resolve to a single transport.
func SingleTransport(ss []string) (a2a.TransportProtocol, error) {
	protos, err := Transports(ss)
	if err != nil {
		return "", err
	}
	if len(protos) != 1 {
		return "", fmt.Errorf("exactly one --transport is required (rest, jsonrpc, grpc, or a plugin name)")
	}
	return protos[0], nil
}

// parseTransport resolves a transport alias. The built-in aliases map to their
// canonical protocols; any other non-empty value is treated as a custom
// (plugin) transport binding and returned verbatim.
func parseTransport(s string) (a2a.TransportProtocol, error) {
	switch strings.ToLower(s) {
	case "":
		return "", fmt.Errorf("empty --transport value")
	case "rest":
		return a2a.TransportProtocol(a2a.TransportProtocolHTTPJSON), nil
	case "jsonrpc":
		return a2a.TransportProtocol(a2a.TransportProtocolJSONRPC), nil
	case "grpc":
		return a2a.TransportProtocol(a2a.TransportProtocolGRPC), nil
	default:
		return a2a.TransportProtocol(s), nil
	}
}
