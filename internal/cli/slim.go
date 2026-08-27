//go:build cgo

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

package cli

import (
	"fmt"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
	a2aslimrpc "github.com/agntcy/slim-a2a-go/a2aslimrpc/v1"
	slim_bindings "github.com/agntcy/slim-bindings-go/v2"
)

func withSLIMRPCTransport(cfg *globalConfig) (a2aclient.FactoryOption, error) {
	localName, err := slim_bindings.NameFromString(cfg.slimLocalName)
	if err != nil {
		return nil, fmt.Errorf("--slim-local-name: %w", err)
	}

	identityProvider, err := slimIdentityProvider(cfg)
	if err != nil {
		return nil, err
	}

	identityVerifier, err := slimIdentityVerifier(cfg)
	if err != nil {
		return nil, err
	}

	slim_bindings.InitializeWithDefaults()
	svc := slim_bindings.GetGlobalService()

	app, err := svc.CreateApp(localName, identityProvider, identityVerifier)
	if err != nil {
		return nil, fmt.Errorf("slim: create app: %w", err)
	}

	connID, err := svc.Connect(slim_bindings.NewInsecureClientConfig(cfg.slimEndpoint))
	if err != nil {
		return nil, fmt.Errorf("slim: connect to %s: %w", cfg.slimEndpoint, err)
	}

	if err := app.Subscribe(localName, &connID); err != nil {
		return nil, fmt.Errorf("slim: subscribe: %w", err)
	}

	cfg.logf("connected to SLIM node at %s as %s", cfg.slimEndpoint, cfg.slimLocalName)

	return a2aslimrpc.WithSLIMRPCTransport(app, &connID), nil
}

func slimIdentityProvider(cfg *globalConfig) (slim_bindings.IdentityProviderConfig, error) {
	switch cfg.slimIdentityProviderType {
	case "sharedSecret":
		return slim_bindings.IdentityProviderConfigSharedSecret{Data: cfg.slimIdentityProviderSharedSecret}, nil
	case "":
		return nil, fmt.Errorf("--slim-identity-provider-type is required with --transport slimrpc (use sharedSecret)")
	default:
		return nil, fmt.Errorf("--slim-identity-provider-type: unknown type %q (use sharedSecret)", cfg.slimIdentityProviderType)
	}
}

func slimIdentityVerifier(cfg *globalConfig) (slim_bindings.IdentityVerifierConfig, error) {
	switch cfg.slimIdentityVerifierType {
	case "sharedSecret":
		return slim_bindings.IdentityVerifierConfigSharedSecret{Data: cfg.slimIdentityVerifierSharedSecret}, nil
	case "":
		return nil, fmt.Errorf("--slim-identity-verifier-type is required with --transport slimrpc (use sharedSecret)")
	default:
		return nil, fmt.Errorf("--slim-identity-verifier-type: unknown type %q (use sharedSecret)", cfg.slimIdentityVerifierType)
	}
}
