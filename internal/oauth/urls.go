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
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	wellKnownServerMetadata   = "/.well-known/oauth-authorization-server"
	wellKnownOpenIDConfig     = "/.well-known/openid-configuration"
	wellKnownResourceMetadata = "/.well-known/oauth-protected-resource"
)

// NormalizeResource returns the canonical form of an agent URL used as an RFC
// 8707 resource indicator and grant-store key: lowercase scheme and host, no
// default port, query, fragment or trailing slash. gRPC "host:port" addresses
// get an https scheme.
func NormalizeResource(raw string) string {
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" && !(scheme == "https" && port == "443") && !(scheme == "http" && port == "80") {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host + strings.TrimSuffix(u.EscapedPath(), "/")
}

// sameIssuer compares issuer identifiers, ignoring a trailing slash.
func sameIssuer(a, b string) bool {
	return strings.TrimSuffix(a, "/") == strings.TrimSuffix(b, "/")
}

// origin returns scheme://host[:port] of a normalized URL.
func origin(raw string) string {
	u, err := url.Parse(NormalizeResource(raw))
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// SameOrigin reports whether two URLs have the same scheme, host and port.
func SameOrigin(a, b string) bool {
	oa := origin(a)
	return oa != "" && oa == origin(b)
}

// resourceCovers reports whether resource identifies target: same origin, and
// the target path is the resource path or below it.
func resourceCovers(resource, target string) bool {
	r, t := NormalizeResource(resource), NormalizeResource(target)
	if origin(r) != origin(t) {
		return false
	}
	rp := strings.TrimPrefix(r, origin(r))
	tp := strings.TrimPrefix(t, origin(t))
	return rp == "" || tp == rp || strings.HasPrefix(tp, rp+"/")
}

// checkSecureURL requires https, allowing http only for loopback hosts so local
// agents and tests work.
func checkSecureURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	switch {
	case u.Host == "":
		return fmt.Errorf("invalid URL %q: missing host", raw)
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && isLoopback(u.Hostname()):
		return nil
	default:
		return fmt.Errorf("refusing non-https URL %q", raw)
	}
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// serverMetadataURLs returns where an issuer's metadata may live: the RFC 8414
// location, with the well-known segment inserted before the issuer path, and the
// OpenID Connect Discovery location, with it appended.
func serverMetadataURLs(issuer string) []string {
	u, err := url.Parse(strings.TrimSuffix(issuer, "/"))
	if err != nil || u.Host == "" {
		return nil
	}
	base := u.Scheme + "://" + u.Host
	path := u.EscapedPath()
	return []string{
		base + wellKnownServerMetadata + path,
		base + path + wellKnownOpenIDConfig,
	}
}

// resourceMetadataURLs returns the RFC 9728 well-known locations for a request
// URL: with the request path inserted, then the origin root.
func resourceMetadataURLs(target *url.URL) []string {
	base := target.Scheme + "://" + target.Host
	path := strings.TrimSuffix(target.EscapedPath(), "/")
	if path == "" {
		return []string{base + wellKnownResourceMetadata}
	}
	return []string{base + wellKnownResourceMetadata + path, base + wellKnownResourceMetadata}
}
