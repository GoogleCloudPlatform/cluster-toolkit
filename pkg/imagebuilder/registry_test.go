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
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestNormalizeRegistryHost(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "GAR hostname", raw: "us-central1-docker.pkg.dev", want: "us-central1-docker.pkg.dev"},
		{name: "IPv4 with port", raw: "10.128.0.68:5000", want: "10.128.0.68:5000"},
		{name: "HTTPS scheme and path stripped", raw: "https://Harbor.Local:8443/ml-team/", want: "harbor.local:8443"},
		{name: "HTTP scheme stripped", raw: "http://localhost:5000/v2/", want: "localhost:5000"},
		{name: "Bracketed IPv6 with port", raw: "[FD12:3456:789a::1]:5000", want: "[fd12:3456:789a::1]:5000"},
		{name: "Bracketed IPv6 without port", raw: "[fd12:3456:789a::1]", want: "[fd12:3456:789a::1]"},
		{name: "Bare IPv6 address bracketed for RFC 3986", raw: "fd12:3456:789a::1", want: "[fd12:3456:789a::1]"},
		{name: "Empty string rejected", raw: "   ", wantErr: true},
		{name: "Invalid authority rejected", raw: "bad host:port", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeRegistryHost(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("normalizeRegistryHost(%q) err = %v, wantErr %v", tc.raw, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("normalizeRegistryHost(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestIsGoogleArtifactRegistryAndPrivateHost(t *testing.T) {
	for _, h := range []string{"us-central1-docker.pkg.dev", "us-docker.pkg.dev", "gcr.io", "us.gcr.io", "https://asia-docker.pkg.dev"} {
		if !isGoogleArtifactRegistry(h) {
			t.Errorf("isGoogleArtifactRegistry(%q) = false, want true", h)
		}
		if isPrivateOrLoopbackHost(h) {
			t.Errorf("isPrivateOrLoopbackHost(%q) = true, want false", h)
		}
	}
	for _, h := range []string{"harbor.local", "10.128.0.68:5000", "[fd12::1]:5000", "localhost:5000"} {
		if isGoogleArtifactRegistry(h) {
			t.Errorf("isGoogleArtifactRegistry(%q) = true, want false", h)
		}
	}
	for _, h := range []string{"localhost", "localhost:5000", "reg.localhost:5000", "127.0.0.1:5000", "[::1]:5000", "10.128.0.68:5000", "172.16.4.2:5000", "192.168.1.9", "[fd12:3456::1]:5000"} {
		if !isPrivateOrLoopbackHost(h) {
			t.Errorf("isPrivateOrLoopbackHost(%q) = false, want true", h)
		}
	}
	if isPrivateOrLoopbackHost("8.8.8.8:5000") || isPrivateOrLoopbackHost("harbor.example.com") {
		t.Errorf("expected public IP and FQDN not to be classified as private/loopback IP")
	}
}

func TestResolveDestinationRepo(t *testing.T) {
	tests := []struct {
		name        string
		project     string
		location    string
		baseImage   string
		envRepo     string
		wantHost    string
		wantPrefix  string
		wantIsGAR   bool
		wantDerived bool
		wantMissing bool
		wantErrSub  string
	}{
		{
			name:       "bare GAR repo name with project and zone",
			project:    "my-proj",
			location:   "us-central1-a",
			baseImage:  "python:3.11-slim",
			envRepo:    "gcluster-repo",
			wantHost:   "us-central1-docker.pkg.dev",
			wantPrefix: "us-central1-docker.pkg.dev/my-proj/gcluster-repo",
			wantIsGAR:  true,
		},
		{
			name:       "nested GAR repo/subpath without registry host preserves develop GAR behavior when project and location are set",
			project:    "my-proj",
			location:   "us-central1-a",
			baseImage:  "python:3.11-slim",
			envRepo:    "gcluster-repo/team-a",
			wantHost:   "us-central1-docker.pkg.dev",
			wantPrefix: "us-central1-docker.pkg.dev/my-proj/gcluster-repo/team-a",
			wantIsGAR:  true,
		},
		{
			name:       "hostless multi-segment GCLUSTER_IMAGE_REPO without project and location fails with Harbor/GHCR and GAR guidance",
			project:    "",
			location:   "",
			baseImage:  "python:3.11-slim",
			envRepo:    "my-proj/my-repo",
			wantErrSub: "'ghcr.io/my-proj/my-repo'",
		},
		{
			name:       "Docker Hub GCLUSTER_IMAGE_REPO with org and repo rejected upfront due to two-level repository path limit",
			baseImage:  "python:3.11-slim",
			envRepo:    "docker.io/my-org/my-repo",
			wantErrSub: "two-level",
		},
		{
			name:       "Docker Hub index.docker.io GCLUSTER_IMAGE_REPO with org only rejected upfront due to two-level repository path limit",
			baseImage:  "python:3.11-slim",
			envRepo:    "index.docker.io/my-org",
			wantErrSub: "two-level",
		},
		{
			name:       "scheme-prefixed hostless GCLUSTER_IMAGE_REPO fails fast",
			project:    "my-proj",
			location:   "us-central1-a",
			baseImage:  "python:3.11-slim",
			envRepo:    "http://my-service/repo",
			wantErrSub: "registry host",
		},
		{
			name:       "empty scheme or slash-only GCLUSTER_IMAGE_REPO fails fast",
			project:    "my-proj",
			location:   "us-central1-a",
			baseImage:  "python:3.11-slim",
			envRepo:    "https://",
			wantErrSub: "cannot be empty",
		},
		{
			name:       "domain-scoped GCP project maps colon to slash in GAR URI",
			project:    "example.com:my-proj",
			location:   "us-central1-a",
			baseImage:  "python:3.11-slim",
			envRepo:    "gcluster-repo",
			wantHost:   "us-central1-docker.pkg.dev",
			wantPrefix: "us-central1-docker.pkg.dev/example.com/my-proj/gcluster-repo",
			wantIsGAR:  true,
		},
		{
			name:       "bare GAR repo without project fails with actionable error",
			project:    "",
			location:   "us-central1",
			baseImage:  "python:3.11-slim",
			envRepo:    "gcluster-repo",
			wantErrSub: "--project and --location",
		},
		{
			name:       "explicit GAR URI in GCLUSTER_IMAGE_REPO",
			project:    "",
			location:   "",
			baseImage:  "python:3.11-slim",
			envRepo:    "us-west4-docker.pkg.dev/my-proj/custom-repo",
			wantHost:   "us-west4-docker.pkg.dev",
			wantPrefix: "us-west4-docker.pkg.dev/my-proj/custom-repo",
			wantIsGAR:  true,
		},
		{
			name:       "explicit domain-scoped GAR URI parses project and repo",
			baseImage:  "python:3.11-slim",
			envRepo:    "us-docker.pkg.dev/example.com/my-proj/custom-repo",
			wantHost:   "us-docker.pkg.dev",
			wantPrefix: "us-docker.pkg.dev/example.com/my-proj/custom-repo",
			wantIsGAR:  true,
		},
		{
			name:       "incomplete domain-scoped GAR URI missing repository segment fails",
			baseImage:  "python:3.11-slim",
			envRepo:    "us-docker.pkg.dev/example.com/my-proj",
			wantErrSub: "<project>/<repository>",
		},
		{
			name:       "Harbor host and mixed-case project in GCLUSTER_IMAGE_REPO lowercased",
			baseImage:  "harbor.local/base/jax:v1",
			envRepo:    "https://harbor.local/ML-Team",
			wantHost:   "harbor.local",
			wantPrefix: "harbor.local/ml-team",
			wantIsGAR:  false,
		},
		{
			name:       "bracketed IPv6 host only in GCLUSTER_IMAGE_REPO defaults repo to library/gcluster",
			baseImage:  "ubuntu:24.04",
			envRepo:    "[fd12:3456::1]:5000",
			wantHost:   "[fd12:3456::1]:5000",
			wantPrefix: "[fd12:3456::1]:5000/library/gcluster",
			wantIsGAR:  false,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO auto-derives library/gcluster from IPv4:port base image",
			baseImage:   "10.128.0.68:5000/ubuntu:24.04",
			wantHost:    "10.128.0.68:5000",
			wantPrefix:  "10.128.0.68:5000/library/gcluster",
			wantIsGAR:   false,
			wantDerived: true,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO auto-derives library/gcluster from bracketed IPv6:port base image",
			baseImage:   "[fd12:3456::1]:5000/tpu/jax:v5p",
			wantHost:    "[fd12:3456::1]:5000",
			wantPrefix:  "[fd12:3456::1]:5000/library/gcluster",
			wantIsGAR:   false,
			wantDerived: true,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO auto-derives library/gcluster from private Harbor hostname base image",
			baseImage:   "harbor.internal.example/ml/jax:v5p@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantHost:    "harbor.internal.example",
			wantPrefix:  "harbor.internal.example/library/gcluster",
			wantIsGAR:   false,
			wantDerived: true,
		},
		{
			name:       "malformed GCLUSTER_IMAGE_REPO with tag colon rejected in ResolveDestinationRepo",
			baseImage:  "python:3.11-slim",
			envRepo:    "harbor.local/ml-team:latest",
			wantErrSub: "invalid destination repository",
		},
		{
			name:       "malformed GCLUSTER_IMAGE_REPO with invalid chars rejected in ResolveDestinationRepo",
			baseImage:  "python:3.11-slim",
			envRepo:    "harbor.local/invalid!!repo",
			wantErrSub: "invalid destination repository",
		},
		{
			name:       "malformed GCLUSTER_IMAGE_REPO with empty double-slash segment rejected",
			baseImage:  "python:3.11-slim",
			envRepo:    "harbor.local/ml-team//sub",
			wantErrSub: "empty path segments",
		},
		{
			name:       "malformed GCLUSTER_IMAGE_REPO with dot-dot segment rejected",
			baseImage:  "python:3.11-slim",
			envRepo:    "harbor.local/ml-team/../other",
			wantErrSub: "'.' or '..'",
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO with Docker Hub shorthand returns ErrMissingImageRepo",
			project:     "my-proj",
			location:    "us-central1",
			baseImage:   "python:3.9-slim",
			wantMissing: true,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO with public nvcr.io returns ErrMissingImageRepo",
			project:     "my-proj",
			location:    "us-central1",
			baseImage:   "nvcr.io/nvidia/pytorch:24.07-py3",
			wantMissing: true,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO with cgr.dev returns ErrMissingImageRepo",
			project:     "my-proj",
			location:    "us-central1",
			baseImage:   "cgr.dev/chainguard/python:latest",
			wantMissing: true,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO with Red Hat registry returns ErrMissingImageRepo",
			project:     "my-proj",
			location:    "us-central1",
			baseImage:   "registry.access.redhat.com/ubi9/python-311:latest",
			wantMissing: true,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO with GitLab registry returns ErrMissingImageRepo",
			project:     "my-proj",
			location:    "us-central1",
			baseImage:   "registry.gitlab.com/group/img:v1",
			wantMissing: true,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO with Azure Container Registry returns ErrMissingImageRepo",
			project:     "my-proj",
			location:    "us-central1",
			baseImage:   "mycorp.azurecr.io/ml/base:v1",
			wantMissing: true,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO with AWS ECR returns ErrMissingImageRepo",
			project:     "my-proj",
			location:    "us-central1",
			baseImage:   "123456789012.dkr.ecr.us-east-1.amazonaws.com/ml/base:v1",
			wantMissing: true,
		},
		{
			name:        "unset GCLUSTER_IMAGE_REPO with GAR base image returns ErrMissingImageRepo",
			project:     "my-proj",
			location:    "us-central1",
			baseImage:   "us-docker.pkg.dev/cloud-tpu-images/jax:v1",
			wantMissing: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveDestinationRepo(tc.project, tc.location, tc.baseImage, tc.envRepo)
			if tc.wantMissing {
				if !errors.Is(err, ErrMissingImageRepo) {
					t.Fatalf("got err = %v, want ErrMissingImageRepo", err)
				}
				return
			}
			if tc.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("got err = %v, want substring %q", err, tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveDestinationRepo() unexpected error: %v", err)
			}
			if got.RegistryHost != tc.wantHost || got.RepoPrefix != tc.wantPrefix || got.IsGAR != tc.wantIsGAR || got.DerivedFromBase != tc.wantDerived {
				t.Errorf("got %+v, want host=%q prefix=%q isGAR=%v derived=%v", got, tc.wantHost, tc.wantPrefix, tc.wantIsGAR, tc.wantDerived)
			}
		})
	}
}

func TestInsecureRegistryScoping(t *testing.T) {
	t.Setenv(insecureRegistryEnvVar, "true")
	platform := &v1.Platform{OS: "linux", Architecture: "amd64"}

	for _, protectedRef := range []string{
		"python:3.11-slim",
		"nvcr.io/nvidia/pytorch:24.07",
		"cgr.dev/chainguard/python:latest",
		"mycorp.azurecr.io/app:v1",
		"123456789012.dkr.ecr.us-east-1.amazonaws.com/app:v1",
		"us-central1-docker.pkg.dev/my-proj/my-repo/img:v1",
	} {
		parsed, err := name.ParseReference(protectedRef, nameOptsForRef(protectedRef)...)
		if err != nil {
			t.Fatalf("ParseReference(%q): %v", protectedRef, err)
		}
		if gotScheme := parsed.Context().Scheme(); gotScheme != "https" {
			t.Errorf("protected ref %q scheme = %q under %s=true, want https", protectedRef, gotScheme, insecureRegistryEnvVar)
		}
		craneOpts := crane.GetOptions(craneOptionsForRef(protectedRef, platform)...)
		if tr, ok := craneOpts.Transport.(*http.Transport); ok && tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
			t.Errorf("protected ref %q enabled InsecureSkipVerify under %s=true", protectedRef, insecureRegistryEnvVar)
		}
	}

	for _, localRef := range []string{
		"harbor.local/ml/img:v1",
		"127.0.0.1:5000/ml/img:v1",
	} {
		parsed, err := name.ParseReference(localRef, nameOptsForRef(localRef)...)
		if err != nil {
			t.Fatalf("ParseReference(%q): %v", localRef, err)
		}
		if gotScheme := parsed.Context().Scheme(); gotScheme != "http" {
			t.Errorf("local ref %q scheme = %q under %s=true, want http", localRef, gotScheme, insecureRegistryEnvVar)
		}
		craneOpts := crane.GetOptions(craneOptionsForRef(localRef, platform)...)
		tr, ok := craneOpts.Transport.(*http.Transport)
		if !ok || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
			t.Errorf("expected local ref %q to enable crane.Insecure transport when %s=true", localRef, insecureRegistryEnvVar)
		}
	}
}
