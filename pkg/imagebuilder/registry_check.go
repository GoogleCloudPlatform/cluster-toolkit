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
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
)

const registryPingTimeout = 2 * time.Second

// InspectTargetRegistry verifies that a non-GAR OCI registry responds to a 2s GET /v2/ ping and has credentials when required.
func (r *RegistryInspector) InspectTargetRegistry(registryAuthority string) error {
	authority, err := normalizeRegistryHost(registryAuthority)
	if err != nil {
		return err
	}
	allowsInsecure := isCustomOCIRegistryHost(authority)
	effectiveInsecure := allowsInsecure && isInsecureRegistryEnv(r.Getenv)
	allowHTTPFallback := effectiveInsecure || isPrivateOrLoopbackHost(authority)

	pingHost := authority
	if pingHost == "docker.io" {
		pingHost = name.DefaultRegistry
	}

	errs, suggestInsecure, done, authErr := r.probeRegistrySchemes(
		r.httpClient(effectiveInsecure),
		authority,
		pingHost,
		allowHTTPFallback,
		allowsInsecure && !effectiveInsecure,
	)
	if done {
		return authErr
	}
	errStr := strings.Join(errs, "; ")
	if suggestInsecure {
		return fmt.Errorf("target container registry %q failed TLS/OCI check (%s); if this is a plain-HTTP or self-signed TLS registry, set %s=true", authority, errStr, insecureRegistryEnvVar)
	}
	return fmt.Errorf("target container registry %q is unreachable or failed OCI /v2/ check (%s)", authority, errStr)
}

func (r *RegistryInspector) probeRegistrySchemes(client *http.Client, authority, pingHost string, allowHTTPFallback, canSuggestInsecure bool) ([]string, bool, bool, error) {
	schemes := []string{"https"}
	if allowHTTPFallback {
		schemes = append(schemes, "http")
	}
	var errs []string
	suggestInsecure := false
	for _, scheme := range schemes {
		status, pingErr := pingRegistryV2(client, scheme, pingHost)
		if pingErr != nil {
			stop, suggest := recordPingError(&errs, scheme, pingErr, allowHTTPFallback, canSuggestInsecure)
			suggestInsecure = suggestInsecure || suggest
			if stop {
				break
			}
			continue
		}
		if status == http.StatusOK {
			return nil, false, true, nil
		}
		if status == http.StatusUnauthorized {
			return nil, false, true, r.verifyRegistryCredentials(authority, allowHTTPFallback)
		}
		errs = append(errs, fmt.Sprintf("%s: unexpected HTTP %d", scheme, status))
		if scheme == "https" {
			break
		}
	}
	return errs, suggestInsecure, false, nil
}

func recordPingError(errs *[]string, scheme string, pingErr error, allowHTTPFallback, canSuggestInsecure bool) (stop, suggest bool) {
	if scheme == "https" {
		if isHTTPSOnPlainHTTPError(pingErr) {
			if allowHTTPFallback {
				return false, false
			}
			*errs = append(*errs, fmt.Sprintf("%s: %v", scheme, pingErr))
			return true, canSuggestInsecure
		}
		if isTLSServerError(pingErr) {
			*errs = append(*errs, fmt.Sprintf("%s: %v", scheme, pingErr))
			return true, canSuggestInsecure
		}
	}
	*errs = append(*errs, fmt.Sprintf("%s: %v", scheme, pingErr))
	return false, false
}

func isHTTPSOnPlainHTTPError(err error) bool {
	if errors.Is(err, http.ErrSchemeMismatch) {
		return true
	}
	var recErr tls.RecordHeaderError
	if errors.As(err, &recErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP response to HTTPS client") || strings.Contains(msg, "first record does not look like a TLS handshake")
}

func isTLSServerError(err error) bool {
	var alertErr tls.AlertError
	if errors.As(err, &alertErr) {
		return true
	}
	var certVerifyErr *tls.CertificateVerificationError
	if errors.As(err, &certVerifyErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "tls:") || strings.Contains(msg, "x509:")
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
	authenticator, err := cmp.Or[authn.Keychain](r.Keychain, defaultKeychain).Resolve(reg)
	if err != nil {
		return fmt.Errorf("failed to resolve Docker credentials for %q: %w", authority, err)
	}
	cfg, err := authenticator.Authorization()
	if err != nil {
		return fmt.Errorf("failed to obtain Docker credentials for %q: %w", authority, err)
	}
	if cfg == nil || *cfg == (authn.AuthConfig{}) {
		return fmt.Errorf("container registry %q requires authentication (HTTP 401 on /v2/) and no credentials were found in Docker config; run 'docker login %s' or 'crane auth login %s' first", authority, authority, authority)
	}
	return nil
}

func (r *RegistryInspector) httpClient(skipVerifyTLS bool) *http.Client {
	tr := cmp.Or(r.Transport, http.DefaultTransport)
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
