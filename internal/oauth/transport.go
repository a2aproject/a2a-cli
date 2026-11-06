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
	"io"
	"net/http"
)

// Transport returns an http.RoundTripper that handles 401 responses to requests
// made through base: it refreshes a rejected grant, or discovers the
// authorization server from the Bearer challenge and uses a stored grant, and
// retries the request once. When a grant has to be obtained interactively, the
// request fails with *InteractionRequiredError. Requests carrying an
// Authorization header the manager did not attach are passed through untouched.
func (m *Manager) Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &transport{base: base, m: m}
}

type transport struct {
	base http.RoundTripper
	m    *Manager
}

// RoundTrip implements http.RoundTripper.
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if req.Header.Get("Authorization") == "" && checkTokenDestination(req.URL.String()) == nil {
		g, err := t.m.learnedGrant(ctx, req.URL)
		if err != nil {
			return nil, err
		}
		if g != nil {
			req = withToken(req, g)
		}
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	chs := ParseChallenges(resp.Header.Values("WWW-Authenticate"))

	if auth := req.Header.Get("Authorization"); auth != "" {
		used, ok := attachedFrom(req.Context())
		if !ok || used.header() != auth {
			return resp, nil
		}
		if bc, ok := bearerChallenge(chs); ok && bc.Params["error"] == "insufficient_scope" {
			return resp, nil
		}
		t.m.logf("the agent rejected the access token for %s; refreshing", used.Key.Resource)
		g, err := t.m.refreshRejected(ctx, used.Key, used.AccessToken)
		if err != nil {
			drain(resp)
			return nil, err
		}
		if g == nil {
			drain(resp)
			return nil, &InteractionRequiredError{Requirement: used.requirement(), Reason: "the stored grant was rejected"}
		}
		return t.retry(req, resp, g)
	}

	if !wantsBearer(chs) {
		return resp, nil
	}
	r, err := t.m.RequirementForChallenge(ctx, req.URL, chs)
	if err != nil {
		drain(resp)
		return nil, err
	}
	if r == nil {
		return resp, nil
	}
	g, err := t.m.Grant(ctx, r)
	if err != nil {
		drain(resp)
		return nil, err
	}
	if g == nil {
		drain(resp)
		return nil, &InteractionRequiredError{Requirement: r}
	}
	t.m.learn(g.Key)
	return t.retry(req, resp, g)
}

// retry repeats req once with the grant's token. A request whose body can't be
// replayed gets the original 401 back.
func (t *transport) retry(req *http.Request, resp *http.Response, g *Grant) (*http.Response, error) {
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return resp, nil
	}
	drain(resp)
	clone := withToken(req, g)
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = body
	}
	return t.base.RoundTrip(clone)
}

// wantsBearer reports whether a 401 may be OAuth-protected: it offers a Bearer
// challenge, or no challenge at all.
func wantsBearer(chs []Challenge) bool {
	if len(chs) == 0 {
		return true
	}
	_, ok := bearerChallenge(chs)
	return ok
}

func withToken(req *http.Request, g *Grant) *http.Request {
	clone := req.Clone(withAttached(req.Context(), g))
	clone.Header.Set("Authorization", g.header())
	return clone
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}
