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
	"context"
	"fmt"
	"net/url"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Requirement describes the grant a request needs: the authorization server to
// get it from, the resource it is for and the scopes it must cover.
type Requirement struct {
	Endpoints Endpoints
	// Resource is the normalized agent URL, sent as the RFC 8707 resource indicator.
	Resource string
	// Scopes requested by the agent; empty when it named none.
	Scopes []string
	// PreferDevice is set when the agent card only offers the device code flow.
	PreferDevice bool
	// Optional is set when the agent card also accepts credentials other than
	// OAuth, such as an API key, so a missing grant is not an error: the request
	// goes out without one and the agent decides.
	Optional bool
}

// requirementFromCard returns the requirement declared by a card for calls to
// baseURL, or nil when the card declares no OAuth requirement the CLI can meet.
// The first securityRequirements option whose only scheme is oauth2 or
// openIdConnect is used; options needing other schemes are skipped, and make
// the requirement optional.
func (f fetcher) requirementFromCard(ctx context.Context, card *a2a.AgentCard, baseURL string) (*Requirement, error) {
	for _, option := range card.SecurityRequirements {
		if len(option) == 0 {
			return nil, nil
		}
		if len(option) != 1 {
			continue
		}
		for name, scopes := range option {
			scheme, ok := card.SecuritySchemes[name]
			if !ok {
				continue
			}
			endpoints, preferDevice, ok, err := f.schemeEndpoints(ctx, scheme)
			if err != nil {
				return nil, fmt.Errorf("security scheme %q: %w", name, err)
			}
			if !ok {
				continue
			}
			return &Requirement{
				Endpoints:    endpoints,
				Resource:     NormalizeResource(baseURL),
				Scopes:       append([]string(nil), scopes...),
				PreferDevice: preferDevice,
				Optional:     acceptsOtherCredentials(card),
			}, nil
		}
	}
	return nil, nil
}

// acceptsOtherCredentials reports whether a card offers a securityRequirements
// option that needs no OAuth scheme, such as an API key or HTTP authentication.
func acceptsOtherCredentials(card *a2a.AgentCard) bool {
	for _, option := range card.SecurityRequirements {
		oauth, defined := false, true
		for name := range option {
			switch card.SecuritySchemes[name].(type) {
			case nil:
				defined = false
			case a2a.OAuth2SecurityScheme, a2a.OpenIDConnectSecurityScheme:
				oauth = true
			}
		}
		if defined && !oauth {
			return true
		}
	}
	return false
}

// schemeEndpoints resolves the endpoints of an oauth2 or openIdConnect scheme.
// It reports ok=false for schemes and flows the CLI can't use.
func (f fetcher) schemeEndpoints(ctx context.Context, scheme a2a.SecurityScheme) (Endpoints, bool, bool, error) {
	switch s := scheme.(type) {
	case a2a.OAuth2SecurityScheme:
		if s.Oauth2MetadataURL != "" {
			m, err := f.metadataAt(ctx, s.Oauth2MetadataURL)
			if err != nil {
				return Endpoints{}, false, false, err
			}
			e := m.endpoints()
			_, device := s.Flows.(a2a.DeviceCodeOAuthFlow)
			return e, device, true, e.validate()
		}
		switch flow := s.Flows.(type) {
		case a2a.AuthorizationCodeOAuthFlow:
			e := Endpoints{Issuer: origin(flow.TokenURL), Authorization: flow.AuthorizationURL, Token: flow.TokenURL}
			return e, false, true, e.validate()
		case a2a.DeviceCodeOAuthFlow:
			e := Endpoints{Issuer: origin(flow.TokenURL), DeviceAuthorization: flow.DeviceAuthorizationURL, Token: flow.TokenURL}
			return e, true, true, e.validate()
		default:
			return Endpoints{}, false, false, nil
		}
	case a2a.OpenIDConnectSecurityScheme:
		if s.OpenIDConnectURL == "" {
			return Endpoints{}, false, false, nil
		}
		m, err := f.metadataAt(ctx, s.OpenIDConnectURL)
		if err != nil {
			return Endpoints{}, false, false, err
		}
		e := m.endpoints()
		return e, false, true, e.validate()
	default:
		return Endpoints{}, false, false, nil
	}
}

// challengeTarget is what a Bearer challenge tells about a 401 response.
type challengeTarget struct {
	resource *ResourceMetadata
	scopes   []string
}

// discoverFromChallenge follows a 401 to the resource's metadata. It returns
// errNoResourceMetadata when the resource doesn't advertise any.
func (f fetcher) discoverFromChallenge(ctx context.Context, target *url.URL, chs []Challenge) (*challengeTarget, error) {
	bc, _ := bearerChallenge(chs)
	prm, err := f.resourceMetadata(ctx, target, bc.Params["resource_metadata"])
	if err != nil {
		return nil, err
	}
	return &challengeTarget{resource: prm, scopes: splitScopes(bc.Params["scope"])}, nil
}
