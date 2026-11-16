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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

	t.Run("docker.io authority rewrites /v2/ ping to index.docker.io", func(t *testing.T) {
		var gotURL string
		insp := &RegistryInspector{
			Getenv: func(string) string { return "" },
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				gotURL = req.URL.String()
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			}),
			Keychain: staticTestKeychain{cfg: authn.AuthConfig{Username: "u", Password: "p"}},
		}
		if err := insp.InspectTargetRegistry("docker.io"); err != nil {
			t.Fatalf("InspectTargetRegistry(docker.io) unexpected error: %v", err)
		}
		if gotURL != "https://index.docker.io/v2/" {
			t.Fatalf("pinged URL = %q, want %q", gotURL, "https://index.docker.io/v2/")
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

	t.Run("self-signed TLS fails without plain-HTTP fallback noise and succeeds when GCLUSTER_INSECURE_REGISTRY=true", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		strictInsp := &RegistryInspector{Getenv: func(string) string { return "" }}
		err := strictInsp.InspectTargetRegistry(u.Host)
		if err == nil || !strings.Contains(err.Error(), InsecureRegistryEnvVar+"=true") {
			t.Fatalf("expected unverified self-signed TLS without %s to fail with hint, got: %v", InsecureRegistryEnvVar, err)
		}
		if strings.Contains(err.Error(), "http:") {
			t.Fatalf("expected TLS verification failure not to send plain-HTTP fallback probe, got: %v", err)
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

	t.Run("plain-HTTP 500 loopback failure omits HTTPS mismatch noise and GCLUSTER_INSECURE_REGISTRY hint", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		insp := &RegistryInspector{Getenv: func(string) string { return "" }}
		err := insp.InspectTargetRegistry(u.Host)
		if err == nil || !strings.Contains(err.Error(), u.Host) || !strings.Contains(err.Error(), "http: unexpected HTTP 500") {
			t.Fatalf("expected plain-HTTP 500 error naming %q, got: %v", u.Host, err)
		}
		if strings.Contains(err.Error(), InsecureRegistryEnvVar) || strings.Contains(err.Error(), "https:") {
			t.Fatalf("did not expect %s hint or https mismatch error on plain-HTTP 500, got: %v", InsecureRegistryEnvVar, err)
		}
		if err := insp.InspectTargetRegistry("   "); err == nil {
			t.Fatalf("expected empty registry host error, got nil")
		}
	})
}

func TestInspectTargetRegistry_HTTPS500NoFallback(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	insp := &RegistryInspector{
		Getenv:    func(string) string { return "" },
		Transport: srv.Client().Transport,
	}
	err := insp.InspectTargetRegistry(u.Host)
	if err == nil || !strings.Contains(err.Error(), "https: unexpected HTTP 500") {
		t.Fatalf("expected https 500 error, got: %v", err)
	}
	if strings.Contains(err.Error(), "http:") || strings.Contains(err.Error(), InsecureRegistryEnvVar) {
		t.Fatalf("did not expect plain-HTTP fallback or insecure hint on valid TLS 500, got: %v", err)
	}
}

func TestInspectTargetRegistry_InsecureScoping(t *testing.T) {
	t.Parallel()

	t.Run("plain-HTTP non-private FQDN suggests GCLUSTER_INSECURE_REGISTRY=true", func(t *testing.T) {
		insp := &RegistryInspector{
			Getenv: func(string) string { return "" },
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, http.ErrSchemeMismatch
			}),
		}
		err := insp.InspectTargetRegistry("harbor.local:5000")
		if err == nil || !strings.Contains(err.Error(), InsecureRegistryEnvVar+"=true") {
			t.Fatalf("expected plain-HTTP non-private FQDN to suggest %s=true, got: %v", InsecureRegistryEnvVar, err)
		}
	})

	t.Run("GAR and public registries keep strict TLS even when GCLUSTER_INSECURE_REGISTRY=true", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		srvURL, _ := url.Parse(srv.URL)

		baseTransport := http.DefaultTransport.(*http.Transport).Clone()
		dialRedirect := &http.Transport{
			TLSClientConfig: baseTransport.TLSClientConfig,
			DialContext:     baseTransport.DialContext,
		}

		for _, protectedHost := range []string{"us-central1-docker.pkg.dev", "nvcr.io", "cgr.dev"} {
			err := inspectProtectedHostAgainstUntrustedTLS(protectedHost, srvURL.Host, dialRedirect)
			if err == nil {
				t.Fatalf("expected strict TLS verification failure for %q despite %s=true", protectedHost, InsecureRegistryEnvVar)
			}
			if strings.Contains(err.Error(), InsecureRegistryEnvVar) || strings.Contains(err.Error(), "http:") {
				t.Fatalf("expected no insecure hint or http fallback for %q, got: %v", protectedHost, err)
			}
		}
	})
}

func inspectProtectedHostAgainstUntrustedTLS(protectedHost, targetAddr string, dialRedirect *http.Transport) error {
	insp := &RegistryInspector{
		Getenv: func(k string) string {
			if k == InsecureRegistryEnvVar {
				return "true"
			}
			return ""
		},
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			cloned := req.Clone(req.Context())
			cloned.URL.Host = targetAddr
			return (&http.Client{Transport: dialRedirect}).Do(cloned)
		}),
	}
	return insp.InspectTargetRegistry(protectedHost)
}
