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

// Package unixsocket routes HTTP requests and gRPC connections through a Unix
// socket.
package unixsocket

import (
	"context"
	"net"
	"net/http"
)

// Transport returns an HTTP transport that sends every request through the
// socket at path, bypassing any proxy.
func Transport(path string) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	dial := Dialer(path)
	t.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		return dial(ctx, addr)
	}
	return t
}

// Dialer returns a dial function, usable with grpc.WithContextDialer, that
// connects to the socket at path whatever the address.
func Dialer(path string) func(context.Context, string) (net.Conn, error) {
	var d net.Dialer
	return func(ctx context.Context, _ string) (net.Conn, error) {
		return d.DialContext(ctx, "unix", path)
	}
}
