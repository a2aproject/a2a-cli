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

package cacert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/a2aproject/a2a-cli/internal/testutil"
)

func TestLoad(t *testing.T) {
	t.Parallel()
	first, second := newCACert(t, "first"), newCACert(t, "second")
	tests := []struct {
		name    string
		path    string
		want    []*x509.Certificate
		wantErr bool
	}{
		{name: "one certificate", path: testutil.MustWriteTmpCertFile(t, first), want: []*x509.Certificate{first}},
		{name: "bundle", path: testutil.MustWriteTmpCertFile(t, first, second), want: []*x509.Certificate{first, second}},
		{name: "no certificates", path: testutil.MustWriteTmpCertFile(t), wantErr: true},
		{name: "missing file", path: filepath.Join(t.TempDir(), "missing.pem"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Load(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Load(%q) error = nil, want error", tc.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(%q) error = %v, want nil", tc.path, err)
			}
			want := x509.NewCertPool()
			for _, cert := range tc.want {
				want.AddCert(cert)
			}
			if !got.Equal(want) {
				t.Fatalf("Load(%q) pool differs from the certificates in the file", tc.path)
			}
		})
	}
}

func newCACert(t *testing.T, name string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() error = %v", err)
	}
	return cert
}
