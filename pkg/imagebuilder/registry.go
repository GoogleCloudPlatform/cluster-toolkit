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
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"hpc-toolkit/pkg/shell"

	"github.com/google/go-containerregistry/pkg/name"
)

const (
	// insecureRegistryEnvVar enables HTTP and unverified TLS connections to custom OCI registries when set to true.
	insecureRegistryEnvVar = "GCLUSTER_INSECURE_REGISTRY"

	// defaultOCIRepoPath is the default repository path used on non-GAR OCI registries (matching Harbor's built-in library project).
	defaultOCIRepoPath = "library/gcluster"
)

// ErrMissingImageRepo indicates that GCLUSTER_IMAGE_REPO is unset and cannot be auto-derived from --base-image.
var ErrMissingImageRepo = errors.New("GCLUSTER_IMAGE_REPO environment variable is required")

var knownPublicRegistries = map[string]bool{
	"ghcr.io":                    true,
	"quay.io":                    true,
	"nvcr.io":                    true,
	"gcr.nvidia.com":             true,
	"mcr.microsoft.com":          true,
	"public.ecr.aws":             true,
	"gallery.ecr.aws":            true,
	"registry.k8s.io":            true,
	"k8s.gcr.io":                 true,
	"cgr.dev":                    true,
	"registry.access.redhat.com": true,
	"registry.redhat.io":         true,
	"registry.gitlab.com":        true,
	"lscr.io":                    true,
	"icr.io":                     true,
}

// WrapDerivedRepoError appends a GCLUSTER_IMAGE_REPO configuration hint when err occurs on an auto-derived destination.
func WrapDerivedRepoError(dest DestinationRepo, err error) error {
	if err == nil || !dest.DerivedFromBase {
		return err
	}
	return fmt.Errorf("%w (destination %q was auto-derived from --base-image because GCLUSTER_IMAGE_REPO is unset; set GCLUSTER_IMAGE_REPO to push to Artifact Registry or another repository)", err, dest.RepoPrefix)
}

// ResolveDestinationRepo resolves the target OCI repository prefix from GCLUSTER_IMAGE_REPO or a non-public/non-GAR --base-image.
func ResolveDestinationRepo(project, location, baseImage, envImageRepo string) (DestinationRepo, error) {
	rawRepo := strings.TrimSpace(envImageRepo)
	if rawRepo != "" {
		return resolveConfiguredRepo(project, location, envImageRepo, rawRepo)
	}
	host, ok := explicitImageHost(baseImage)
	if ok && isCustomOCIRegistryHost(host) {
		return validateDestinationRepo(DestinationRepo{
			RegistryHost:    host,
			RepoPrefix:      host + "/" + defaultOCIRepoPath,
			IsGAR:           false,
			DerivedFromBase: true,
		})
	}
	return DestinationRepo{}, fmt.Errorf("%w. Please set it in your environment (e.g., export GCLUSTER_IMAGE_REPO=<repo>)", ErrMissingImageRepo)
}

func normalizeRegistryHost(raw string) (string, error) {
	s := stripRegistryScheme(strings.TrimSpace(raw))
	authority, _, _ := strings.Cut(s, "/")
	if authority == "" {
		return "", fmt.Errorf("registry host cannot be empty")
	}
	normalized, err := normalizeAuthorityHostPort(authority)
	if err != nil {
		return "", err
	}
	u, err := url.Parse("//" + normalized)
	if err != nil || u.Host != normalized || strings.ContainsAny(normalized, " \t\r\n@?#") {
		return "", fmt.Errorf("invalid registry authority %q", raw)
	}
	return normalized, nil
}

func stripRegistryScheme(s string) string {
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "https://"):
		return s[len("https://"):]
	case strings.HasPrefix(lower, "http://"):
		return s[len("http://"):]
	default:
		return s
	}
}

func normalizeAuthorityHostPort(authority string) (string, error) {
	if strings.HasPrefix(authority, "[") {
		if strings.HasSuffix(authority, "]") {
			inner := strings.Trim(authority, "[]")
			if net.ParseIP(inner) == nil {
				return "", fmt.Errorf("invalid IPv6 registry host %q", authority)
			}
			return "[" + strings.ToLower(inner) + "]", nil
		}
		h, p, err := net.SplitHostPort(authority)
		if err != nil || net.ParseIP(h) == nil || p == "" {
			return "", fmt.Errorf("invalid bracketed registry host:port %q", authority)
		}
		return net.JoinHostPort(strings.ToLower(h), p), nil
	}
	if ip := net.ParseIP(authority); ip != nil && strings.Contains(authority, ":") {
		return "[" + strings.ToLower(authority) + "]", nil
	}
	if strings.Contains(authority, ":") {
		h, p, err := net.SplitHostPort(authority)
		if err != nil || h == "" || p == "" {
			return "", fmt.Errorf("invalid registry host:port %q", authority)
		}
		return net.JoinHostPort(strings.ToLower(h), p), nil
	}
	return strings.ToLower(authority), nil
}

func registryBareHost(raw string) string {
	norm, err := normalizeRegistryHost(raw)
	if err != nil {
		norm = strings.ToLower(strings.TrimSpace(raw))
	}
	if h, _, splitErr := net.SplitHostPort(norm); splitErr == nil {
		return h
	}
	return strings.Trim(norm, "[]")
}

func isGoogleArtifactRegistry(host string) bool {
	bare := registryBareHost(host)
	return bare == "gcr.io" || strings.HasSuffix(bare, ".gcr.io") || strings.HasSuffix(bare, ".pkg.dev")
}

func isPrivateOrLoopbackHost(host string) bool {
	bare := registryBareHost(host)
	if bare == "localhost" || strings.HasSuffix(bare, ".localhost") {
		return true
	}
	if ip := net.ParseIP(bare); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate()
	}
	return false
}

func isDockerHubHost(host string) bool {
	bare := registryBareHost(host)
	return bare == "docker.io" || strings.HasSuffix(bare, ".docker.io")
}

func isPublicRegistry(host string) bool {
	bare := registryBareHost(host)
	return isDockerHubHost(bare) ||
		knownPublicRegistries[bare] ||
		strings.HasSuffix(bare, ".azurecr.io") ||
		strings.HasSuffix(bare, ".amazonaws.com") ||
		strings.HasSuffix(bare, ".amazonaws.com.cn")
}

func isCustomOCIRegistryHost(host string) bool {
	return host != "" && !isGoogleArtifactRegistry(host) && !isPublicRegistry(host)
}

func isInsecureRegistryEnv(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	switch strings.ToLower(strings.TrimSpace(getenv(insecureRegistryEnvVar))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func hasRegistryHostPrefix(seg string) bool {
	return strings.EqualFold(seg, "localhost") || strings.ContainsAny(seg, ".:")
}

func explicitImageHost(imageRef string) (string, bool) {
	withoutDigest, _, _ := strings.Cut(strings.TrimSpace(imageRef), "@")
	first, rest, hasSlash := strings.Cut(withoutDigest, "/")
	if !hasSlash || rest == "" || !hasRegistryHostPrefix(first) {
		return "", false
	}
	norm, err := normalizeRegistryHost(first)
	if err != nil {
		return "", false
	}
	return norm, true
}

func allowsInsecureRegistryRef(ref string, getenv func(string) string) bool {
	if !isInsecureRegistryEnv(getenv) {
		return false
	}
	host, ok := explicitImageHost(ref)
	return ok && isCustomOCIRegistryHost(host)
}

func resolveConfiguredRepo(project, location, envImageRepo, rawRepo string) (DestinationRepo, error) {
	lowerRaw := strings.ToLower(rawRepo)
	hadScheme := strings.HasPrefix(lowerRaw, "http://") || strings.HasPrefix(lowerRaw, "https://")
	trimmedRepo := strings.Trim(stripRegistryScheme(rawRepo), "/")
	if trimmedRepo == "" {
		return DestinationRepo{}, fmt.Errorf("invalid GCLUSTER_IMAGE_REPO %q: repository cannot be empty", envImageRepo)
	}
	first, _, _ := strings.Cut(trimmedRepo, "/")
	if hadScheme && !hasRegistryHostPrefix(first) {
		return DestinationRepo{}, fmt.Errorf("invalid GCLUSTER_IMAGE_REPO %q: registry host %q must be 'localhost' or contain '.' or ':'", envImageRepo, first)
	}
	dest, err := resolveExplicitRepo(project, location, trimmedRepo)
	if err != nil {
		return DestinationRepo{}, err
	}
	return validateDestinationRepo(dest)
}

func parseExplicitGARProject(repoPath string) (string, bool) {
	parts := strings.Split(repoPath, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	if strings.Contains(parts[0], ".") {
		if len(parts) < 3 || parts[2] == "" {
			return "", false
		}
		return parts[0] + ":" + parts[1], true
	}
	return parts[0], true
}

func resolveExplicitRepo(project, location, trimmedRepo string) (DestinationRepo, error) {
	first, rest, _ := strings.Cut(trimmedRepo, "/")
	if hasRegistryHostPrefix(first) {
		host, err := normalizeRegistryHost(first)
		if err != nil {
			return DestinationRepo{}, err
		}
		repoPath := strings.ToLower(strings.Trim(rest, "/"))
		if isGoogleArtifactRegistry(host) {
			garProj, ok := parseExplicitGARProject(repoPath)
			if !ok {
				return DestinationRepo{}, fmt.Errorf("GCLUSTER_IMAGE_REPO %q must include <project>/<repository> when specifying an Artifact Registry host", trimmedRepo)
			}
			return DestinationRepo{RegistryHost: host, RepoPrefix: host + "/" + repoPath, GARProject: garProj, IsGAR: true}, nil
		}
		if repoPath == "" {
			repoPath = defaultOCIRepoPath
		}
		return DestinationRepo{RegistryHost: host, RepoPrefix: host + "/" + repoPath, IsGAR: false}, nil
	}
	if strings.TrimSpace(project) == "" || strings.TrimSpace(location) == "" {
		if strings.Contains(trimmedRepo, "/") {
			return DestinationRepo{}, fmt.Errorf("GCLUSTER_IMAGE_REPO=%q has no registry host and requires --project and --location for Artifact Registry (or include a registry host such as 'harbor.example.com/%s' or 'ghcr.io/%s')", trimmedRepo, trimmedRepo, trimmedRepo)
		}
		return DestinationRepo{}, fmt.Errorf("GCLUSTER_IMAGE_REPO=%q is a bare Artifact Registry repository name and requires --project and --location (or set GCLUSTER_IMAGE_REPO=<registry-host>/<repository> for a non-Artifact Registry OCI registry)", trimmedRepo)
	}
	host := shell.ExtractRegion(location) + "-docker.pkg.dev"
	// Domain-scoped GCP project IDs (example.com:my-proj) map ":" to "/" in Artifact Registry paths.
	garProject := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(project)), ":", "/")
	return DestinationRepo{
		RegistryHost: host,
		RepoPrefix:   fmt.Sprintf("%s/%s/%s", host, garProject, strings.ToLower(trimmedRepo)),
		GARProject:   strings.TrimSpace(project),
		IsGAR:        true,
	}, nil
}

func validateDestinationRepo(dest DestinationRepo) (DestinationRepo, error) {
	if isDockerHubHost(dest.RegistryHost) {
		return DestinationRepo{}, fmt.Errorf("invalid destination repository %q: Docker Hub only allows two-level '<namespace>/<repository>' paths, whereas gcluster pushes to '<repository-prefix>/<namespace>/<workload>'; use a registry that supports nested repository paths (such as Artifact Registry, Harbor, or ghcr.io)", dest.RepoPrefix)
	}
	_, repoPath, hasSlash := strings.Cut(dest.RepoPrefix, "/")
	if !hasSlash || repoPath == "" {
		return DestinationRepo{}, fmt.Errorf("invalid destination repository %q: repository path cannot be empty", dest.RepoPrefix)
	}
	if strings.Contains(repoPath, "//") {
		return DestinationRepo{}, fmt.Errorf("invalid destination repository %q: repository path cannot contain empty path segments", dest.RepoPrefix)
	}
	for _, seg := range strings.Split(repoPath, "/") {
		if seg == "." || seg == ".." {
			return DestinationRepo{}, fmt.Errorf("invalid destination repository %q: repository path cannot contain '.' or '..' segments", dest.RepoPrefix)
		}
	}
	if _, err := name.NewRepository(dest.RepoPrefix, nameOptsForRef(dest.RepoPrefix)...); err != nil {
		return DestinationRepo{}, fmt.Errorf("invalid destination repository %q: %w", dest.RepoPrefix, err)
	}
	return dest, nil
}
