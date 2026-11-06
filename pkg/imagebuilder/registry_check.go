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
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
)

const registryPingTimeout = 2 * time.Second

// RegistryInspector verifies OCI Distribution /v2/ reachability and keychain credentials with injectable test seams.
type RegistryInspector struct {
	Getenv    func(string) string
	Transport http.RoundTripper
	Keychain  authn.Keychain
}

// InspectTargetRegistry verifies that registryAuthority responds to a bounded 2s GET /v2/ ping and has Docker keychain credentials when required.
func InspectTargetRegistry(registryAuthority string) error {
	return (&RegistryInspector{}).InspectTargetRegistry(registryAuthority)
}

// InspectTargetRegistry verifies that a non-GAR OCI registry responds to a 2s GET /v2/ ping and has credentials when required.
func (r *RegistryInspector) InspectTargetRegistry(registryAuthority string) error {
	authority, err := NormalizeRegistryHost(registryAuthority)
	if err != nil {
		return err
	}
	insecureEnv := IsInsecureRegistryEnv(r.getenv())
	allowHTTPFallback := insecureEnv || IsPrivateOrLoopbackHost(authority)

	schemes := []string{"https"}
	if allowHTTPFallback {
		schemes = append(schemes, "http")
	}

	client := r.httpClient(insecureEnv)

	var errs []string
	for _, scheme := range schemes {
		status, pingErr := pingRegistryV2(client, scheme, authority)
		if pingErr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", scheme, pingErr))
			continue
		}
		if status == http.StatusOK {
			return nil
		}
		if status == http.StatusUnauthorized {
			return r.verifyRegistryCredentials(authority, allowHTTPFallback)
		}
		errs = append(errs, fmt.Sprintf("%s: unexpected HTTP %d", scheme, status))
	}
	errStr := strings.Join(errs, "; ")
	if !insecureEnv && (strings.Contains(errStr, "tls:") || strings.Contains(errStr, "x509:") || strings.Contains(errStr, "HTTP response to HTTPS client")) {
		return fmt.Errorf("target container registry %q failed TLS/OCI check (%s); if this is a plain-HTTP or self-signed TLS registry, set %s=true", authority, errStr, InsecureRegistryEnvVar)
	}
	return fmt.Errorf("target container registry %q is unreachable or failed OCI /v2/ check (%s)", authority, errStr)
}

func pingRegistryV2(client *http.Client, scheme, authority string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), registryPingTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s://%s/v2/", scheme, authority), nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func (r *RegistryInspector) verifyRegistryCredentials(authority string, allowInsecureName bool) error {
	var opts []name.Option
	if allowInsecureName {
		opts = append(opts, name.Insecure)
	}
	reg, err := name.NewRegistry(authority, opts...)
	if err != nil {
		return fmt.Errorf("invalid target registry %q: %w", authority, err)
	}
	authenticator, err := r.keychain().Resolve(reg)
	if err != nil {
		return fmt.Errorf("failed to resolve Docker credentials for %q: %w", authority, err)
	}
	cfg, err := authenticator.Authorization()
	if err != nil {
		return fmt.Errorf("failed to obtain Docker credentials for %q: %w", authority, err)
	}
	if cfg == nil || *cfg == (authn.AuthConfig{}) {
		return fmt.Errorf("container registry %q requires authentication (HTTP 401 on /v2/) and no credentials were found in Docker config; run 'docker login %s' first", authority, authority)
	}
	return nil
}

func (r *RegistryInspector) getenv() func(string) string {
	if r.Getenv != nil {
		return r.Getenv
	}
	return os.Getenv
}

func (r *RegistryInspector) keychain() authn.Keychain {
	if r.Keychain != nil {
		return r.Keychain
	}
	return defaultKeychain
}

func (r *RegistryInspector) httpClient(skipVerifyTLS bool) *http.Client {
	tr := r.Transport
	if tr == nil {
		tr = http.DefaultTransport
	}
	if skipVerifyTLS {
		tr = cloneTransportWithInsecureTLS(tr)
	}
	return &http.Client{Transport: tr}
}

func cloneTransportWithInsecureTLS(rt http.RoundTripper) http.RoundTripper {
	if t, ok := rt.(*http.Transport); ok {
		cloned := t.Clone()
		if cloned.TLSClientConfig == nil {
			cloned.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit opt-in via GCLUSTER_INSECURE_REGISTRY=true
		} else {
			cloned.TLSClientConfig = cloned.TLSClientConfig.Clone()
			cloned.TLSClientConfig.InsecureSkipVerify = true //nolint:gosec // explicit opt-in via GCLUSTER_INSECURE_REGISTRY=true
		}
		return cloned
	}
	return rt
}
