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

package auth

import (
	"context"

	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// callUserKey is the attribute of the A2A call user which holds the principal.
const callUserKey = "principal"

// SetCallUser makes p the user of an A2A call.
func SetCallUser(cc *a2asrv.CallContext, p Principal) {
	cc.User = a2asrv.NewAuthenticatedUser(p.ID, map[string]any{callUserKey: p})
}

// Caller returns the principal of a request: the one the HTTP layer put in
// ctx, or the user of the A2A call.
func Caller(ctx context.Context) (Principal, bool) {
	if p, ok := PrincipalFrom(ctx); ok {
		return p, true
	}
	if cc, ok := a2asrv.CallContextFrom(ctx); ok && cc.User != nil {
		if p, ok := cc.User.Attributes[callUserKey].(Principal); ok {
			return p, true
		}
	}
	return Principal{}, false
}

// InProcess returns a context for calling an A2A handler directly as p.
func InProcess(ctx context.Context, p Principal) context.Context {
	ctx = WithPrincipal(ctx, p)
	ctx, cc := a2asrv.NewCallContext(ctx, a2asrv.NewServiceParams(nil))
	SetCallUser(cc, p)
	return ctx
}
