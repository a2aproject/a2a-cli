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
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

type attachedGrantKey struct{}

// withAttached records on ctx the grant whose token a request carries, so the
// transport can tell its own token from an explicit credential.
func withAttached(ctx context.Context, g *Grant) context.Context {
	return context.WithValue(ctx, attachedGrantKey{}, g)
}

func attachedFrom(ctx context.Context) (*Grant, bool) {
	g, ok := ctx.Value(attachedGrantKey{}).(*Grant)
	return g, ok
}

// Interceptor returns an a2a client interceptor that attaches grants to calls.
// When the agent card requires OAuth for the call's interface, the call fails
// with *InteractionRequiredError if no grant is stored. Otherwise a stored grant
// is attached when there is one, and problems finding it are only logged, so
// agents that accept other credentials or none are not affected. Calls that
// already carry an Authorization service parameter (from --auth, --svc-param or
// a credential helper) are left alone.
func (m *Manager) Interceptor() a2aclient.CallInterceptor {
	return &interceptor{m: m}
}

type interceptor struct {
	a2aclient.PassthroughInterceptor
	m *Manager
}

var _ a2aclient.CallInterceptor = (*interceptor)(nil)

// Before implements [a2aclient.CallInterceptor].
func (i *interceptor) Before(ctx context.Context, req *a2aclient.Request) (context.Context, any, error) {
	if len(req.ServiceParams.Get("authorization")) > 0 {
		return ctx, nil, nil
	}
	var r *Requirement
	if req.Card != nil {
		var err error
		r, err = i.m.RequirementForCard(ctx, req.Card, req.BaseURL)
		if err != nil {
			if !acceptsOtherCredentials(req.Card) {
				return ctx, nil, err
			}
			i.m.logf("not using OAuth for %s: %v", req.BaseURL, err)
		}
	}
	if r == nil || r.Optional {
		g, err := i.optionalGrant(ctx, req.BaseURL, r)
		if err != nil || g == nil {
			if err != nil {
				i.m.logf("not using a stored OAuth grant for %s: %v", req.BaseURL, err)
			}
			return ctx, nil, nil
		}
		req.ServiceParams.Append("Authorization", g.header())
		return withAttached(ctx, g), nil, nil
	}
	if err := checkTokenDestination(req.BaseURL); err != nil {
		return ctx, nil, failure("%w", err)
	}
	g, err := i.m.Grant(ctx, r)
	if err != nil {
		return ctx, nil, err
	}
	if g == nil {
		return ctx, nil, &InteractionRequiredError{Requirement: r}
	}
	req.ServiceParams.Append("Authorization", g.header())
	return withAttached(ctx, g), nil, nil
}

// optionalGrant returns a stored grant for a call that doesn't require one: for
// the optional requirement r, or else for the call's resource.
func (i *interceptor) optionalGrant(ctx context.Context, baseURL string, r *Requirement) (*Grant, error) {
	if err := checkTokenDestination(baseURL); err != nil {
		return nil, err
	}
	if r != nil {
		return i.m.Grant(ctx, r)
	}
	return i.m.GrantForResource(ctx, baseURL)
}

// checkTokenDestination refuses to send a bearer token where it would travel in
// cleartext: it allows https, http to a loopback host, and gRPC addresses
// (host:port), which use TLS unless --insecure is set.
func checkTokenDestination(target string) error {
	if !strings.Contains(target, "://") {
		return nil
	}
	if err := checkSecureURL(target); err != nil {
		return fmt.Errorf("refusing to send an OAuth token to %s over plain HTTP", target)
	}
	return nil
}
