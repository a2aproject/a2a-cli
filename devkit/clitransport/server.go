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

package clitransport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

const (
	healthReadyTimeout  = 10 * time.Second
	healthProbeTimeout  = 2 * time.Second
	healthRetryInterval = 25 * time.Millisecond
)

type serverCore struct {
	address         string
	serveFunc       func(ctx context.Context) error
	healthcheckFunc func(ctx context.Context) error
}

type server struct {
	body      *Endpoint
	core      *serverCore
	cancel    context.CancelFunc
	serveErr  error
	serveDone chan struct{}
}

func newServer(binding a2a.TransportProtocol, token string, handler a2asrv.RequestHandler) (*server, error) {
	tlsSetup, err := genLoopbackTLSSetup()
	if err != nil {
		return nil, fmt.Errorf("generating loopback certificate: %w", err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	var srvCore *serverCore
	switch binding {
	case a2a.TransportProtocolHTTPJSON, a2a.TransportProtocolJSONRPC:
		core, err := newHTTPServerCore(lis, binding, handler, tlsSetup)
		if err != nil {
			return nil, err
		}
		srvCore = core
	case a2a.TransportProtocolGRPC:
		core, err := newGRPCServerCore(lis, handler, tlsSetup)
		if err != nil {
			return nil, err
		}
		srvCore = core
	default:
		return nil, fmt.Errorf("unsupported binding %q (want %s, %s or %s)", binding, a2a.TransportProtocolGRPC, a2a.TransportProtocolHTTPJSON, a2a.TransportProtocolJSONRPC)
	}

	return &server{
		core:      srvCore,
		serveDone: make(chan struct{}),
		body: &Endpoint{
			Address:  srvCore.address,
			Binding:  binding,
			Protocol: a2a.Version,
			Token:    token,
			CertPEM:  string(tlsSetup.certPEM),
		},
	}, nil
}

func newHTTPServerCore(lis net.Listener, binding a2a.TransportProtocol, handler a2asrv.RequestHandler, tlsSetup *tlsSetup) (*serverCore, error) {
	mux := http.NewServeMux()
	switch binding {
	case a2a.TransportProtocolHTTPJSON:
		mux.Handle("/", a2asrv.NewRESTHandler(handler))
	case a2a.TransportProtocolJSONRPC:
		mux.Handle("/", a2asrv.NewJSONRPCHandler(handler))
	default:
		return nil, fmt.Errorf("unexpected binding %q (want %s or %s)", binding, a2a.TransportProtocolHTTPJSON, a2a.TransportProtocolJSONRPC)
	}

	srv := http.Server{
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{tlsSetup.serverCert}},
	}

	healthClient := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsSetup.clientConf}}
	address := "https://" + lis.Addr().String()
	return &serverCore{
		address: address,

		serveFunc: func(ctx context.Context) error {
			go func() {
				<-ctx.Done()
				_ = srv.Shutdown(context.Background())
			}()
			if err := srv.ServeTLS(lis, "", ""); err != nil && err != http.ErrServerClosed {
				return err
			}
			return nil
		},

		healthcheckFunc: func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, address+a2asrv.WellKnownAgentCardPath, nil)
			if err != nil {
				return err
			}
			resp, err := healthClient.Do(req)
			if err != nil {
				return err
			}
			return resp.Body.Close()
		},
	}, nil
}

func newGRPCServerCore(lis net.Listener, handler a2asrv.RequestHandler, tlsSetup *tlsSetup) (*serverCore, error) {
	serverTLS := &tls.Config{Certificates: []tls.Certificate{tlsSetup.serverCert}}
	grpcSrv := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	a2agrpc.NewHandler(handler).RegisterWith(grpcSrv)

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	address := lis.Addr().String()
	return &serverCore{
		address: address,

		serveFunc: func(ctx context.Context) error {
			go func() {
				<-ctx.Done()
				grpcSrv.GracefulStop()
			}()
			return grpcSrv.Serve(lis)
		},

		healthcheckFunc: func(ctx context.Context) error {
			conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(tlsSetup.clientConf)))
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()
			_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{}, grpc.WaitForReady(true))
			return err
		},
	}, nil
}

// start runs the server in the background and blocks until it becomes healthy.
// The server keeps running until ctx is cancelled or [server.stop] is called.
func (s *server) start(ctx context.Context) error {
	serveCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go func() {
		s.serveErr = s.core.serveFunc(serveCtx)
		close(s.serveDone)
	}()

	if err := s.waitHealthy(ctx); err != nil {
		cancel()
		<-s.serveDone
		return err
	}

	return nil
}

func (s *server) await() error {
	<-s.serveDone
	return s.serveErr
}

func (s *server) stop() {
	s.cancel()
	<-s.serveDone
}

func (s *server) waitHealthy(ctx context.Context) error {
	deadline := time.NewTimer(healthReadyTimeout)
	defer deadline.Stop()

	var lastErr error
	for {
		select {
		case <-s.serveDone:
			return fmt.Errorf("server exited before becoming ready: %w", s.serveErr)
		default:
		}

		probeCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
		lastErr = s.core.healthcheckFunc(probeCtx)
		cancel()
		if lastErr == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-deadline.C:
			return fmt.Errorf("server did not become ready within %s: %w", healthReadyTimeout, lastErr)

		case <-s.serveDone:
			return fmt.Errorf("server exited before becoming ready: %w", s.serveErr)

		case <-time.After(healthRetryInterval):
		}
	}
}
