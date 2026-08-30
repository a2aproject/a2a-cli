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

package clitransport

import "github.com/a2aproject/a2a-go/v2/a2a"

// Plugin subcommands a plugin binary must implement.
const (
	// SubcommandServe starts the loopback proxy server, effectively opening a transport.
	SubcommandServe = "serve"
	// SubcommandInfo prints an [Info] JSON document describing the plugin.
	SubcommandInfo = "info"
)

// TokenSvcParam is the service parameter carrying the per-launch shared secret for the
// jsonrpc and rest bindings. The host sets it on every request; the plugin
// rejects requests without a matching value.
const TokenSvcParam = "A2A-Plugin-Token"

// Handshake is the single JSON line a plugin prints to stdout after it has
// attempted to start its loopback proxy server.
type Handshake struct {
	// Success reports whether the proxy server started and passed its readiness check.
	Success bool `json:"success"`
	// Error describes why the plugin failed to start. Empty on success.
	Error string `json:"error,omitempty"`
	// Payload carries the connection details. Non-nil only on success.
	Payload *HandshakeBody `json:"payload,omitempty"`
}

// HandshakeBody carries the details the host needs to connect to a plugin's
// loopback proxy server.
type HandshakeBody struct {
	// Address is the loopback address of the proxy server.
	Address string `json:"address"`
	// Binding names the standard A2A transport binding the proxy speaks.
	Binding a2a.TransportProtocol `json:"binding"`
	// Protocol is the A2A protocol version the proxy exposes (e.g. "1.0").
	Protocol a2a.ProtocolVersion `json:"protocol"`
	// Token is the per-launch shared secret the host must present on every call.
	Token string `json:"token"`
	// CertPEM is the PEM-encoded, self-signed certificate the proxy server
	// presents for TLS. When non-empty, the host connects over TLS and pins this
	// certificate as the only trusted root.
	CertPEM string `json:"certPem,omitempty"`
}

// Info describes a transport plugin. It is printed by the "info" subcommand.
type Info struct {
	// Name is the transport name.
	Name string `json:"name"`
	// Version is the plugin's own version string.
	Version string `json:"version"`
	// Description is a short human-readable summary of the transport.
	Description string `json:"description,omitempty"`
	// Protocol is the A2A protocol version the plugin targets (e.g. "1.0").
	Protocol a2a.ProtocolVersion `json:"protocol,omitempty"`
	// Binding is the default loopback binding the plugin serves.
	Binding a2a.TransportProtocol `json:"binding,omitempty"`
}
