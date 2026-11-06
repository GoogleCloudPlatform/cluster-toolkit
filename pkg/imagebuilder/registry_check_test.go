// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package imagebuilder

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
)

type staticTestKeychain struct {
	cfg authn.AuthConfig
}

func (k staticTestKeychain) Resolve(authn.Resource) (authn.Authenticator, error) {
	if k.cfg == (authn.AuthConfig{}) {
		return authn.Anonymous, nil
	}
	return authn.FromConfig(k.cfg), nil
}

type errAuthenticator struct {
	err error
}

func (a errAuthenticator) Authorization() (*authn.AuthConfig, error) {
	return nil, a.err
}

type errKeychain struct {
	resolveErr error
	authErr    error
}

func (k errKeychain) Resolve(authn.Resource) (authn.Authenticator, error) {
	if k.resolveErr != nil {
		return nil, k.resolveErr
	}
	return errAuthenticator{err: k.authErr}, nil
}

func TestInspectTargetRegistry_OpenAndAuthKeychain(t *testing.T) {
	t.Parallel()

	t.Run("open 200 OK HTTP loopback registry succeeds without credentials", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v2/" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		insp := &RegistryInspector{
			Getenv:   func(string) string { return "" },
			Keychain: staticTestKeychain{},
		}
		if err := insp.InspectTargetRegistry(u.Host); err != nil {
			t.Fatalf("InspectTargetRegistry(%q) unexpected error: %v", u.Host, err)
		}
	})

	t.Run("401 Unauthorized requires keychain credentials and surfaces helper errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		anonInsp := &RegistryInspector{
			Getenv:   func(string) string { return "" },
			Keychain: staticTestKeychain{},
		}
		if err := anonInsp.InspectTargetRegistry(u.Host); err == nil || !strings.Contains(err.Error(), "docker login") {
			t.Fatalf("expected missing-credentials docker login error on 401, got: %v", err)
		}

		brokenHelper := &RegistryInspector{
			Getenv:   func(string) string { return "" },
			Keychain: errKeychain{authErr: errors.New("credential helper crashed")},
		}
		if err := brokenHelper.InspectTargetRegistry(u.Host); err == nil || !strings.Contains(err.Error(), "credential helper crashed") {
			t.Fatalf("expected credential helper error propagation, got: %v", err)
		}

		authedInsp := &RegistryInspector{
			Getenv:   func(string) string { return "" },
			Keychain: staticTestKeychain{cfg: authn.AuthConfig{Username: "ci", Password: "secret"}},
		}
		if err := authedInsp.InspectTargetRegistry(u.Host); err != nil {
			t.Fatalf("expected 401 with configured keychain to pass, got: %v", err)
		}
	})
}

func TestInspectTargetRegistry_TLSAndHTTPErrors(t *testing.T) {
	t.Parallel()

	t.Run("self-signed TLS succeeds when GCLUSTER_INSECURE_REGISTRY=true and fails otherwise", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		strictInsp := &RegistryInspector{Getenv: func(string) string { return "" }}
		if err := strictInsp.InspectTargetRegistry(u.Host); err == nil || !strings.Contains(err.Error(), InsecureRegistryEnvVar+"=true") {
			t.Fatalf("expected unverified self-signed TLS without %s to fail with hint, got: %v", InsecureRegistryEnvVar, err)
		}

		insecureInsp := &RegistryInspector{
			Getenv: func(k string) string {
				if k == InsecureRegistryEnvVar {
					return "true"
				}
				return ""
			},
		}
		if err := insecureInsp.InspectTargetRegistry(u.Host); err != nil {
			t.Fatalf("expected %s=true to accept self-signed TLS, got: %v", InsecureRegistryEnvVar, err)
		}
	})

	t.Run("non-OCI 500 error and invalid host fail fast", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		insp := &RegistryInspector{Getenv: func(string) string { return "" }}
		if err := insp.InspectTargetRegistry(u.Host); err == nil || !strings.Contains(err.Error(), u.Host) {
			t.Fatalf("expected 500 error naming %q, got: %v", u.Host, err)
		}
		if err := insp.InspectTargetRegistry("   "); err == nil {
			t.Fatalf("expected empty registry host error, got nil")
		}
	})
}
