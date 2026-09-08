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

// Package clitransport is a helper for authoring A2A CLI transport plugins.
//
// A transport plugin is a standalone binary named "a2a-transport-<name>" discoverable
// through PATH. When the a2a CLI needs a transport it does not implement natively, it
// launches the plugin as a subprocess. The plugin starts a loopback proxy server that speaks
// a standard A2A binding (jsonrpc, rest or grpc) and forwards every request through the
// custom binding transport.
//
// A minimal plugin looks like:
//
//	func main() {
//		clitransport.Main(clitransport.Config{
//			Name:    "carrier-pigeon",
//			Version: "1.0.0",
//			NewTransport: func(ctx context.Context, endpoint string) (a2aclient.Transport, error) {
//				return pigeon.NewSender(endpoint)
//			},
//		})
//	}
package clitransport
