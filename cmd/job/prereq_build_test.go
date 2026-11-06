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
	"encoding/base64"
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

func TestEnsureBuildPrerequisites_ExplicitGARSkipsGKEAuthPlugin(t *testing.T) {
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

	t.Setenv("GCLUSTER_IMAGE_REPO", "us-west4-docker.pkg.dev/gar-build-proj/custom-repo")
	writeDockerConfig(t, "us-west4-docker.pkg.dev")

	cmd := &cobra.Command{}
	if err := ensureBuildPrerequisites(cmd, "", ""); err != nil {
		t.Fatalf("explicit GAR ensureBuildPrerequisites failed: %v", err)
	}
	if !ranArtifactRegistryCheck(*executed) {
		t.Errorf("expected Artifact Registry API check against gar-build-proj, executed = %v", *executed)
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

		if err := ensureBuildPrerequisites(cmd, "", ""); err == nil || !strings.Contains(err.Error(), "docker login") {
			t.Fatalf("expected docker login error when 401 registry lacks keychain creds, got: %v", err)
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
