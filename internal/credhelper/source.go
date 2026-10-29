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

package credhelper

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// Source runs the helper selected for each URI and reuses its headers for
// that URI until they expire.
type Source struct {
	Helpers []Helper
	// Logf, if set, is called before a helper runs.
	Logf func(format string, args ...any)

	mu    sync.Mutex
	cache map[string]*Response
}

// Headers returns the selected helper and its response for uri, or a nil
// response when no helper's scope matches.
func (s *Source) Headers(ctx context.Context, uri string) (Helper, *Response, error) {
	h, ok := Select(s.Helpers, uri)
	if !ok {
		return Helper{}, nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if resp, ok := s.cache[uri]; ok && !expired(resp) {
		return h, resp, nil
	}
	if s.Logf != nil {
		s.Logf("asking credential helper %q for %s", h.Spec, uri)
	}
	resp, err := h.Get(ctx, uri)
	if err != nil {
		return h, nil, fmt.Errorf("getting credentials for %s: %w", uri, err)
	}
	if s.cache == nil {
		s.cache = map[string]*Response{}
	}
	s.cache[uri] = resp
	return h, resp, nil
}

func expired(resp *Response) bool {
	if resp.Expires == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, resp.Expires)
	return err != nil || !time.Now().Before(t)
}

// Interceptor adds the headers from the helper for each call's interface URL
// as service parameters, unless the call already carries the same parameter.
// The helper receives the interface URL rather than the URL of the method,
// which the interceptor does not see.
type Interceptor struct {
	a2aclient.PassthroughInterceptor
	Source *Source
	// OnApply, if set, is called with the headers added to a call.
	OnApply func(headers map[string][]string)
}

var _ a2aclient.CallInterceptor = (*Interceptor)(nil)

// Before implements [a2aclient.CallInterceptor].
func (i *Interceptor) Before(ctx context.Context, req *a2aclient.Request) (context.Context, any, error) {
	_, resp, err := i.Source.Headers(ctx, interfaceURI(req.BaseURL))
	if err != nil || resp == nil {
		return ctx, nil, err
	}
	applied := map[string][]string{}
	for name, values := range resp.Headers {
		if len(req.ServiceParams.Get(name)) > 0 {
			continue
		}
		req.ServiceParams.Append(name, values...)
		applied[name] = values
	}
	if i.OnApply != nil && len(applied) > 0 {
		i.OnApply(applied)
	}
	return ctx, nil, nil
}

// interfaceURI gives gRPC "host:port" addresses a scheme, so that helpers
// receive a URI and the scope is matched against the host.
func interfaceURI(baseURL string) string {
	if strings.Contains(baseURL, "://") {
		return baseURL
	}
	return "https://" + baseURL
}
