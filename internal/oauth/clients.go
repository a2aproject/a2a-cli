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

package oauth

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DefaultClientID is the Client ID Metadata Document published for a2a-cli. It is
// used with authorization servers that support CIMD when no registration matches.
const DefaultClientID = "https://a2a-protocol.org/oauth/a2a-cli.json"

// Client sources reported by [ClientChoice].
const (
	ClientSourceConfig = "config"
	ClientSourceCIMD   = "cimd-default"
)

// Registration is a client registration from configuration.
type Registration struct {
	// Authority is the issuer identifier of the authorization server.
	Authority string `json:"authority"`
	// ClientID is a pre-registered client ID or a Client ID Metadata Document URL.
	ClientID string `json:"client_id"`
	// Scopes are requested when the agent names none.
	Scopes []string `json:"scopes,omitempty"`
}

// ClientChoice is the client used with an authorization server.
type ClientChoice struct {
	Registration
	// Source is ClientSourceConfig or ClientSourceCIMD.
	Source string
}

// ParseRegistrations parses --oauth-clients values. Each value is a JSON object
// or a JSON array of objects.
func ParseRegistrations(values []string) ([]Registration, error) {
	var regs []Registration
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		var batch []Registration
		if strings.HasPrefix(v, "[") {
			if err := json.Unmarshal([]byte(v), &batch); err != nil {
				return nil, fmt.Errorf("invalid oauth-clients value %q: %w", v, err)
			}
		} else {
			var r Registration
			if err := json.Unmarshal([]byte(v), &r); err != nil {
				return nil, fmt.Errorf("invalid oauth-clients value %q: want a JSON object with authority and client_id: %w", v, err)
			}
			batch = []Registration{r}
		}
		for _, r := range batch {
			if err := r.validate(); err != nil {
				return nil, err
			}
			regs = append(regs, r)
		}
	}
	return regs, nil
}

func (r Registration) validate() error {
	if r.Authority == "" || r.ClientID == "" {
		return fmt.Errorf("invalid oauth-clients entry %+v: authority and client_id are required", r)
	}
	if err := checkSecureURL(r.Authority); err != nil {
		return fmt.Errorf("invalid oauth-clients authority: %w", err)
	}
	return nil
}

// selectClient picks the registration for an authorization server, falling back
// to defaultClientID, normally [DefaultClientID], when the server supports Client
// ID Metadata Documents. An empty defaultClientID disables the fallback.
func selectClient(regs []Registration, defaultClientID string, e Endpoints) (ClientChoice, error) {
	var matches []Registration
	for _, r := range regs {
		if sameIssuer(r.Authority, e.Issuer) {
			matches = append(matches, r)
		}
	}
	switch {
	case len(matches) == 1:
		return ClientChoice{Registration: matches[0], Source: ClientSourceConfig}, nil
	case len(matches) > 1:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ClientID
		}
		return ClientChoice{}, fmt.Errorf("several oauth-clients entries match %s (%s); keep one", e.Issuer, strings.Join(ids, ", "))
	case e.CIMDSupported && defaultClientID != "":
		return ClientChoice{Registration: Registration{Authority: e.Issuer, ClientID: defaultClientID}, Source: ClientSourceCIMD}, nil
	default:
		return ClientChoice{}, &NoClientError{Issuers: []string{e.Issuer}}
	}
}
