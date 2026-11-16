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

package job

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hpc-toolkit/pkg/shell"

	"github.com/spf13/cobra"
)

func writeDockerConfig(t *testing.T, registries ...string) {
	t.Helper()
	helpers := map[string]string{}
	for _, r := range registries {
		helpers[r] = "gcloud"
	}
	data, err := json.Marshal(map[string]any{"credHelpers": helpers})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("DOCKER_CONFIG"), "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func ranArtifactRegistryCheck(executed []string) bool {
	for _, c := range executed {
		if strings.HasPrefix(c, "gcloud services list") {
			return true
		}
	}
	return false
}

func TestCheckArtifactRegistryAPI_Timeout(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		if timeout != shell.DefaultCloudAPITimeout {
			t.Errorf("expected timeout %v, got %v", shell.DefaultCloudAPITimeout, timeout)
		}
		return shell.CommandResult{ExitCode: 124, Err: context.DeadlineExceeded}
	}

	var state PrereqState
	var missing []missingPrereq

	checkArtifactRegistryAPI("test-project", &state, &missing)

	if len(missing) != 1 {
		t.Fatalf("expected 1 missing prereq, got %d", len(missing))
	}

	expectedName := fmt.Sprintf("Artifact Registry API (verification timed out after %v)", shell.DefaultCloudAPITimeout)
	if missing[0].name != expectedName {
		t.Errorf("missing[0].name = %q, want %q", missing[0].name, expectedName)
	}
	if state.ArtifactRegistryAPIEnabled {
		t.Error("expected ArtifactRegistryAPIEnabled to remain false on timeout")
	}
}

func TestIsDockerCredsConfigured(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	t.Setenv("DOCKER_CONFIG", "")

	dockerDir := filepath.Join(tempDir, ".docker")
	configPath := filepath.Join(dockerDir, "config.json")
	const registry = "us-central1-docker.pkg.dev"

	tests := []struct {
		name          string
		configContent string
		shouldExist   bool
		expected      bool
	}{
		{name: "No config file", shouldExist: false, expected: false},
		{name: "Corrupted config file", configContent: "invalid", shouldExist: true, expected: false},
		{name: "CredsStore set to gcloud", configContent: `{"credsStore": "gcloud"}`, shouldExist: true, expected: true},
		{name: "CredHelpers configured for gcr.io and the regional registry", configContent: `{"credHelpers": {"gcr.io": "gcloud", "us-central1-docker.pkg.dev": "gcloud"}}`, shouldExist: true, expected: true},
		{name: "CredHelpers configured for the regional registry only", configContent: `{"credHelpers": {"us-central1-docker.pkg.dev": "gcloud"}}`, shouldExist: true, expected: true},
		{name: "CredHelpers missing regional registry", configContent: `{"credHelpers": {"gcr.io": "gcloud"}}`, shouldExist: true, expected: false},
		{name: "CredHelpers wrong helper for the regional registry", configContent: `{"credHelpers": {"gcr.io": "gcloud", "us-central1-docker.pkg.dev": "desktop"}}`, shouldExist: true, expected: false},
		{name: "CredHelpers overrides CredsStore for the regional registry", configContent: `{"credsStore": "gcloud", "credHelpers": {"us-central1-docker.pkg.dev": "desktop"}}`, shouldExist: true, expected: false},
		{name: "CredHelpers set to gcr for the regional registry", configContent: `{"credHelpers": {"us-central1-docker.pkg.dev": "gcr"}}`, shouldExist: true, expected: true},
		{name: "CredsStore set to gcr", configContent: `{"credsStore": "gcr"}`, shouldExist: true, expected: true},
		{name: "Auths entry for the regional registry", configContent: `{"auths": {"us-central1-docker.pkg.dev": {"auth": "b2F1dGgyYWNjZXNzdG9rZW46dG9rZW4="}}}`, shouldExist: true, expected: true},
		{name: "Auths entry with https prefix for the regional registry", configContent: `{"auths": {"https://us-central1-docker.pkg.dev": {"auth": "b2F1dGgyYWNjZXNzdG9rZW46dG9rZW4="}}}`, shouldExist: true, expected: true},
		{name: "Empty auths entry for the regional registry", configContent: `{"auths": {"us-central1-docker.pkg.dev": {}}}`, shouldExist: true, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.shouldExist {
				_ = os.Remove(configPath)
			} else {
				if err := os.MkdirAll(dockerDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configPath, []byte(tt.configContent), 0644); err != nil {
					t.Fatal(err)
				}
			}

			got := isDockerCredsConfigured(registry)
			if got != tt.expected {
				t.Errorf("isDockerCredsConfigured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEnsureBuildPrerequisites_MissingDockerCreds(t *testing.T) {
	stubPrereqCommands(t)
	t.Setenv("GCLUSTER_IMAGE_REPO", "test-repo")

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := ensureBuildPrerequisites(cmd, "test-project", "us-central1-a")
	if err == nil {
		t.Fatalf("expected an error without Docker credentials, got nil\noutput:\n%s", buf.String())
	}
	output := buf.String()
	if want := "gcloud auth configure-docker us-central1-docker.pkg.dev --quiet"; !strings.Contains(output, want) {
		t.Errorf("expected output to contain %q, got:\n%s", want, output)
	}
	if strings.Contains(output, "gcr.io") {
		t.Errorf("a Crane build only pushes to Artifact Registry; output must not ask for gcr.io credentials:\n%s", output)
	}
}

func TestEnsureBuildPrerequisites_FreshState(t *testing.T) {
	tests := []struct {
		name           string
		dockerRegistry string
		wantErr        bool
	}{
		{name: "credentials for the cluster's region skip all checks", dockerRegistry: "us-west4-docker.pkg.dev"},
		{name: "credentials for another region only are not enough", dockerRegistry: "us-central1-docker.pkg.dev", wantErr: true},
		{name: "no Docker config", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executed := stubPrereqCommands(t)
			t.Setenv("GCLUSTER_IMAGE_REPO", "test-repo")
			if tt.dockerRegistry != "" {
				writeDockerConfig(t, tt.dockerRegistry)
			}

			store.Save(PrereqState{
				GCloudSDKInstalled:           true,
				GCloudProjectConfigured:      true,
				GCloudAuthenticated:          true,
				ADCConfigured:                true,
				KubectlInstalled:             true,
				GKEGCloudAuthPluginInstalled: true,
				ArtifactRegistryAPIEnabled:   true,
				LastCheckedProjectID:         "test-project",
				LastCheckedTimestamp:         time.Now(),
			})

			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			err := ensureBuildPrerequisites(cmd, "test-project", "us-west4-a")
			if (err != nil) != tt.wantErr {
				t.Fatalf("ensureBuildPrerequisites() error = %v, wantErr %v\noutput:\n%s", err, tt.wantErr, buf.String())
			}
			if got := strings.Contains(buf.String(), "gcloud auth configure-docker us-west4-docker.pkg.dev --quiet"); got != tt.wantErr {
				t.Errorf("remediation for us-west4 shown = %v, want %v\noutput:\n%s", got, tt.wantErr, buf.String())
			}
			if ran := len(*executed) > 0; ran != tt.wantErr {
				t.Errorf("executed %v; want checks to run = %v", *executed, tt.wantErr)
			}
		})
	}
}

func TestEnsureBuildPrerequisites_ExplicitGARSkipsGKEAuthPluginAndPreservesCache(t *testing.T) {
	setupSubmitTestEnv(t)
	executed := stubPrereqCommands(t)
	origExec := shell.ExecuteCommandWithTimeout
	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		if name == "gke-gcloud-auth-plugin" {
			t.Errorf("Artifact Registry Crane build must not require gke-gcloud-auth-plugin")
			return shell.CommandResult{ExitCode: 127, Stderr: "not installed"}
		}
		return origExec(timeout, name, args...)
	}

	// Pre-populate unexpired GKE tool states for cluster-proj; switching to gar-build-proj must preserve them.
	store.Save(PrereqState{
		GCloudSDKInstalled:           true,
		GCloudAuthenticated:          true,
		ADCConfigured:                true,
		KubectlInstalled:             true,
		GKEGCloudAuthPluginInstalled: true,
		LastCheckedProjectID:         "cluster-proj",
		LastCheckedTimestamp:         time.Now(),
	})

	t.Setenv("GCLUSTER_IMAGE_REPO", "us-west4-docker.pkg.dev/gar-build-proj/custom-repo")
	writeDockerConfig(t, "us-west4-docker.pkg.dev")

	cmd := &cobra.Command{}
	if err := ensureBuildPrerequisites(cmd, "", ""); err != nil {
		t.Fatalf("explicit GAR ensureBuildPrerequisites failed: %v", err)
	}
	if !ranArtifactRegistryCheck(*executed) {
		t.Errorf("expected Artifact Registry API check against gar-build-proj, executed = %v", *executed)
	}
	saved := store.Load()
	if !saved.KubectlInstalled || !saved.GKEGCloudAuthPluginInstalled {
		t.Fatalf("expected project switch to preserve unexpired KubectlInstalled and GKEGCloudAuthPluginInstalled, got %+v", saved)
	}

	expired := resetProjectScopedPrereqState(PrereqState{
		GCloudSDKInstalled:           true,
		GCloudAuthenticated:          true,
		ADCConfigured:                true,
		KubectlInstalled:             true,
		GKEGCloudAuthPluginInstalled: true,
		ArtifactRegistryAPIEnabled:   true,
		LastCheckedProjectID:         "gar-build-proj",
		LastCheckedTimestamp:         time.Now().Add(-2 * stateFreshness),
	}, "gar-build-proj")
	if !expired.GCloudSDKInstalled || !expired.KubectlInstalled || !expired.GKEGCloudAuthPluginInstalled {
		t.Fatalf("expected TTL expiry to preserve installed CLI binary flags, got %+v", expired)
	}
	if expired.GCloudAuthenticated || expired.ADCConfigured || expired.ArtifactRegistryAPIEnabled {
		t.Fatalf("expected TTL expiry to force re-verification of auth and API flags, got %+v", expired)
	}
}

func TestEnsureBuildPrerequisites_CrossProjectGARPermissionDeniedProceeds(t *testing.T) {
	setupSubmitTestEnv(t)
	_ = stubPrereqCommands(t)
	origExec := shell.ExecuteCommandWithTimeout
	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		if name == "gcloud" && len(args) > 1 && args[0] == "services" && args[1] == "list" {
			return shell.CommandResult{ExitCode: 1, Stderr: "ERROR: (gcloud.services.list) PERMISSION_DENIED: Permission denied to list services"}
		}
		return origExec(timeout, name, args...)
	}

	t.Setenv("GCLUSTER_IMAGE_REPO", "us-west4-docker.pkg.dev/shared-host-proj/team-repo")
	writeDockerConfig(t, "us-west4-docker.pkg.dev")

	cmd := &cobra.Command{}
	if err := ensureBuildPrerequisites(cmd, "", ""); err != nil {
		t.Fatalf("expected cross-project GAR PERMISSION_DENIED on services.list to warn and proceed, got: %v", err)
	}
}

func TestEnsureBuildPrerequisites_NonGARLocalRegistry(t *testing.T) {
	t.Run("open 200 OK /v2/ registry succeeds with zero gcloud calls", func(t *testing.T) {
		setupSubmitTestEnv(t)
		t.Cleanup(func() { baseImage = "" })
		executed := stubPrereqCommands(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		baseImage = u.Host + "/base:v1"
		t.Setenv("GCLUSTER_IMAGE_REPO", "")
		cmd := &cobra.Command{}
		if err := ensureBuildPrerequisites(cmd, "", ""); err != nil {
			t.Fatalf("ensureBuildPrerequisites(open local registry) error = %v", err)
		}
		for _, c := range *executed {
			if strings.HasPrefix(c, "gcloud") {
				t.Errorf("unexpected gcloud invocation during non-GAR build prereqs: %q", c)
			}
		}
	})

	t.Run("401 /v2/ registry checks Docker keychain without gcloud", func(t *testing.T) {
		setupSubmitTestEnv(t)
		t.Cleanup(func() { baseImage = "" })
		executed := stubPrereqCommands(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		baseImage = u.Host + "/base:v1"
		t.Setenv("GCLUSTER_IMAGE_REPO", "")
		cmd := &cobra.Command{}

		if err := ensureBuildPrerequisites(cmd, "", ""); err == nil || !strings.Contains(err.Error(), "docker login") || !strings.Contains(err.Error(), "GCLUSTER_IMAGE_REPO") {
			t.Fatalf("expected docker login + GCLUSTER_IMAGE_REPO hint when auto-derived 401 registry lacks keychain creds, got: %v", err)
		}

		token := base64.StdEncoding.EncodeToString([]byte("ci:secret"))
		cfgBytes := []byte(`{"auths":{"` + u.Host + `":{"auth":"` + token + `"}}}`)
		if err := os.WriteFile(filepath.Join(os.Getenv("DOCKER_CONFIG"), "config.json"), cfgBytes, 0600); err != nil {
			t.Fatal(err)
		}
		if err := ensureBuildPrerequisites(cmd, "", ""); err != nil {
			t.Fatalf("expected 401 registry with DOCKER_CONFIG auth to pass, got: %v", err)
		}
		for _, c := range *executed {
			if strings.HasPrefix(c, "gcloud") {
				t.Errorf("unexpected gcloud invocation during non-GAR build prereqs: %q", c)
			}
		}
	})
}

func assertNoGCloudAuthSubprocesses(t *testing.T, executed []string) {
	t.Helper()
	for _, c := range executed {
		if strings.HasPrefix(c, "gcloud version") || strings.HasPrefix(c, "gcloud auth") {
			t.Fatalf("expected cached gcloud SDK/auth to be reused, got executed=%v", executed)
		}
	}
}

func TestCrossProjectGARAndBasicPrerequisites_NoCacheThrashing(t *testing.T) {
	setupSubmitTestEnv(t)
	executed := stubPrereqCommands(t)
	t.Setenv("GCLUSTER_IMAGE_REPO", "us-west4-docker.pkg.dev/gar-build-proj/custom-repo")
	writeDockerConfig(t, "us-west4-docker.pkg.dev")

	cmd := &cobra.Command{}
	if err := ensureBasicPrerequisites(cmd, "cluster-proj"); err != nil {
		t.Fatalf("first ensureBasicPrerequisites failed: %v", err)
	}

	*executed = nil
	if err := ensureBuildPrerequisites(cmd, "cluster-proj", "us-central1-a"); err != nil {
		t.Fatalf("first cross-project ensureBuildPrerequisites failed: %v", err)
	}
	assertNoGCloudAuthSubprocesses(t, *executed)
	if !ranArtifactRegistryCheck(*executed) {
		t.Fatalf("expected Artifact Registry check for gar-build-proj on first build, executed=%v", *executed)
	}

	saved := store.Load()
	if saved.LastCheckedProjectID != "cluster-proj" || !saved.GCloudProjectConfigured || saved.LastCheckedARProjectID != "gar-build-proj" {
		t.Fatalf("expected both cluster and AR projects cached independently, got %+v", saved)
	}

	*executed = nil
	if err := ensureBasicPrerequisites(cmd, "cluster-proj"); err != nil {
		t.Fatalf("second ensureBasicPrerequisites failed: %v", err)
	}
	if err := ensureBuildPrerequisites(cmd, "cluster-proj", "us-central1-a"); err != nil {
		t.Fatalf("second ensureBuildPrerequisites failed: %v", err)
	}
	if len(*executed) != 0 {
		t.Fatalf("expected zero subprocess checks on cached repeat run, got executed=%v", *executed)
	}
}

func TestBuildPrerequisitesBeforeBasicPrerequisites_ReusesAuth(t *testing.T) {
	setupSubmitTestEnv(t)
	executed := stubPrereqCommands(t)
	t.Setenv("GCLUSTER_IMAGE_REPO", "us-west4-docker.pkg.dev/gar-build-proj/custom-repo")
	writeDockerConfig(t, "us-west4-docker.pkg.dev")

	cmd := &cobra.Command{}
	if err := ensureBuildPrerequisites(cmd, "", ""); err != nil {
		t.Fatalf("ensureBuildPrerequisites failed: %v", err)
	}

	*executed = nil
	if err := ensureBasicPrerequisites(cmd, "gar-build-proj"); err != nil {
		t.Fatalf("ensureBasicPrerequisites after build prereqs failed: %v", err)
	}
	assertNoGCloudAuthSubprocesses(t, *executed)

	joined := "\n" + strings.Join(*executed, "\n")
	for _, want := range []string{"\ngcloud projects describe gar-build-proj", "\nkubectl ", "\ngke-gcloud-auth-plugin "} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in executed commands, got %v", strings.TrimSpace(want), *executed)
		}
	}
}
