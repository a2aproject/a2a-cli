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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxMetadataBytes = 1 << 20

// ServerMetadata is the subset of RFC 8414 Authorization Server Metadata and
// OpenID Connect Discovery the CLI uses.
type ServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint                     string   `json:"token_endpoint,omitempty"`
	DeviceAuthorizationEndpoint       string   `json:"device_authorization_endpoint,omitempty"`
	RevocationEndpoint                string   `json:"revocation_endpoint,omitempty"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported,omitempty"`
	ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported,omitempty"`
}

// ResourceMetadata is the subset of RFC 9728 Protected Resource Metadata the CLI uses.
type ResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers,omitempty"`
	ScopesSupported      []string `json:"scopes_supported,omitempty"`
}

// Endpoints are the authorization server endpoints a grant is obtained,
// refreshed and revoked with.
type Endpoints struct {
	Issuer              string `json:"issuer"`
	Authorization       string `json:"authorization,omitempty"`
	Token               string `json:"token"`
	DeviceAuthorization string `json:"device_authorization,omitempty"`
	Revocation          string `json:"revocation,omitempty"`
	// CIMDSupported reports that the server accepts Client ID Metadata Document URLs as client IDs.
	CIMDSupported bool `json:"cimd_supported,omitempty"`
}

func (m *ServerMetadata) endpoints() Endpoints {
	return Endpoints{
		Issuer:              m.Issuer,
		Authorization:       m.AuthorizationEndpoint,
		Token:               m.TokenEndpoint,
		DeviceAuthorization: m.DeviceAuthorizationEndpoint,
		Revocation:          m.RevocationEndpoint,
		CIMDSupported:       m.ClientIDMetadataDocumentSupported,
	}
}

func (e Endpoints) validate() error {
	if e.Issuer == "" {
		return errors.New("authorization server has no issuer")
	}
	if e.Token == "" {
		return fmt.Errorf("authorization server %s has no token endpoint", e.Issuer)
	}
	for _, u := range []string{e.Authorization, e.Token, e.DeviceAuthorization, e.Revocation} {
		if u == "" {
			continue
		}
		if err := checkSecureURL(u); err != nil {
			return fmt.Errorf("authorization server %s: %w", e.Issuer, err)
		}
	}
	if e.Authorization == "" && e.DeviceAuthorization == "" {
		return fmt.Errorf("authorization server %s has neither an authorization nor a device authorization endpoint", e.Issuer)
	}
	return nil
}

// fetcher makes the CLI's own requests to metadata and authorization server
// endpoints. Its client never carries agent credentials.
type fetcher struct {
	client *http.Client
}

// getJSON fetches a JSON document over https (or loopback http). With
// sameOrigin, redirects are only followed within the URL's origin.
func (f fetcher) getJSON(ctx context.Context, rawURL string, sameOrigin bool, out any) error {
	if err := checkSecureURL(rawURL); err != nil {
		return err
	}
	client := *f.client
	start := origin(rawURL)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if sameOrigin && origin(req.URL.String()) != start {
			return fmt.Errorf("redirect from %s to another origin %s refused", rawURL, req.URL)
		}
		return checkSecureURL(req.URL.String())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &statusError{url: rawURL, status: resp.StatusCode}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxMetadataBytes)).Decode(out); err != nil {
		return fmt.Errorf("parsing %s: %w", rawURL, err)
	}
	return nil
}

type statusError struct {
	url    string
	status int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d", e.url, e.status)
}

// serverMetadata fetches and validates the metadata of an issuer, trying the RFC
// 8414 location first and OpenID Connect Discovery second.
func (f fetcher) serverMetadata(ctx context.Context, issuer string) (*ServerMetadata, error) {
	urls := serverMetadataURLs(issuer)
	if len(urls) == 0 {
		return nil, fmt.Errorf("invalid authorization server identifier %q", issuer)
	}
	var errs []error
	for _, u := range urls {
		var m ServerMetadata
		if err := f.getJSON(ctx, u, false, &m); err != nil {
			errs = append(errs, err)
			continue
		}
		if !sameIssuer(m.Issuer, issuer) {
			return nil, fmt.Errorf("metadata at %s names issuer %q, want %q", u, m.Issuer, issuer)
		}
		return &m, nil
	}
	return nil, fmt.Errorf("fetching metadata of %s: %w", issuer, errors.Join(errs...))
}

// metadataAt fetches metadata from a URL given by an agent card and checks that
// the URL is a well-known location of the issuer it names.
func (f fetcher) metadataAt(ctx context.Context, rawURL string) (*ServerMetadata, error) {
	var m ServerMetadata
	if err := f.getJSON(ctx, rawURL, false, &m); err != nil {
		return nil, err
	}
	for _, u := range serverMetadataURLs(m.Issuer) {
		if strings.TrimSuffix(u, "/") == strings.TrimSuffix(rawURL, "/") {
			return &m, nil
		}
	}
	return nil, fmt.Errorf("metadata at %s names issuer %q, which does not publish metadata there", rawURL, m.Issuer)
}

// resourceMetadata fetches the Protected Resource Metadata for a request that got
// a 401: from the challenge's resource_metadata, or else from the well-known
// locations. It returns errNoResourceMetadata when the resource publishes none.
func (f fetcher) resourceMetadata(ctx context.Context, target *url.URL, metadataURL string) (*ResourceMetadata, error) {
	candidates := []string{metadataURL}
	explicit := metadataURL != ""
	if !explicit {
		candidates = resourceMetadataURLs(target)
	}
	var errs []error
	for _, u := range candidates {
		var m ResourceMetadata
		err := f.getJSON(ctx, u, true, &m)
		if err != nil {
			var se *statusError
			if !explicit && errors.As(err, &se) && se.status == http.StatusNotFound {
				continue
			}
			errs = append(errs, err)
			continue
		}
		if m.Resource == "" {
			return nil, fmt.Errorf("resource metadata at %s has no resource", u)
		}
		if !resourceCovers(m.Resource, target.String()) {
			return nil, fmt.Errorf("resource metadata at %s describes %s, not %s", u, m.Resource, target)
		}
		if len(m.AuthorizationServers) == 0 {
			return nil, fmt.Errorf("resource metadata at %s lists no authorization servers", u)
		}
		return &m, nil
	}
	if len(errs) == 0 {
		return nil, errNoResourceMetadata
	}
	return nil, errors.Join(errs...)
}

var errNoResourceMetadata = errors.New("no protected resource metadata")
