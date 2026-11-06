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

// Package oauthstore keeps the CLI's OAuth configuration and grants: the
// --oauth-clients registrations, the default client ID and the grant file in
// the user's config directory.
package oauthstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

// DefaultClientID is the Client ID Metadata Document published for a2a-cli. It is
// used with authorization servers that support CIMD when no registration matches.
const DefaultClientID = "https://a2a-protocol.org/oauth/a2a-cli.json"

// ParseRegistrations parses --oauth-clients values. Each value is a JSON object
// or a JSON array of objects.
func ParseRegistrations(values []string) ([]oauth.Registration, error) {
	var regs []oauth.Registration
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		var batch []oauth.Registration
		if strings.HasPrefix(v, "[") {
			if err := decodeStrict(v, &batch); err != nil {
				return nil, fmt.Errorf("invalid oauth-clients value %q: %w", v, err)
			}
		} else {
			var r oauth.Registration
			if err := decodeStrict(v, &r); err != nil {
				return nil, fmt.Errorf("invalid oauth-clients value %q: want a JSON object with authority and client_id: %w", v, err)
			}
			batch = []oauth.Registration{r}
		}
		for _, r := range batch {
			if err := validate(r); err != nil {
				return nil, err
			}
			regs = append(regs, r)
		}
	}
	return regs, nil
}

// decodeStrict decodes a JSON value, rejecting unknown fields, so a misspelled
// key such as "omit-resource" is reported rather than ignored, as it is in
// config files.
func decodeStrict(v string, dst any) error {
	dec := json.NewDecoder(strings.NewReader(v))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}

func validate(r oauth.Registration) error {
	if r.Authority == "" || r.ClientID == "" {
		return fmt.Errorf("invalid oauth-clients entry %+v: authority and client_id are required", r)
	}
	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid oauth-clients entry: %w", err)
	}
	return nil
}

// NoClientMessage explains an [oauth.NoClientError] in terms of --oauth-clients.
func NoClientMessage(err error) (string, bool) {
	var nc *oauth.NoClientError
	if !errors.As(err, &nc) {
		return "", false
	}
	example := `{"authority":"<issuer>","client_id":"<client id or CIMD URL>"}`
	if len(nc.Issuers) > 0 {
		example = fmt.Sprintf(`{"authority":%q,"client_id":"<client id or CIMD URL>"}`, nc.Issuers[0])
	}
	return fmt.Sprintf("no client in oauth-clients matches the authorization server %s; add %s to oauth-clients",
		strings.Join(nc.Issuers, ", "), example), true
}
