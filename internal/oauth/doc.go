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

// Package oauth obtains, stores and attaches OAuth access tokens for agents.
//
// The CLI learns how to authorize from two sources: the agent card's
// securitySchemes and securityRequirements, read before the first request, and
// Bearer challenges on a 401 response, which lead to Protected Resource Metadata
// (RFC 9728) and Authorization Server Metadata (RFC 8414). Grants come from the
// authorization code flow with PKCE and a loopback redirect (RFC 7636, RFC 8252)
// or the device authorization grant (RFC 8628), and are kept in a [Store] keyed
// by issuer, client ID, resource and scopes.
package oauth
