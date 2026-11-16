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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"
)

func setupBuildContextEnv(t *testing.T) {
	t.Helper()
	setupSubmitTestEnv(t)
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	t.Setenv("GCLUSTER_INSECURE_REGISTRY", "")
	t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
	t.Cleanup(func() {
		baseImage = ""
		buildContext = ""
	})
}

func TestValidateBuildContext_OCIRegistries(t *testing.T) {
	t.Run("private local base-image succeeds with empty GCLUSTER_IMAGE_REPO and empty USER", func(t *testing.T) {
		setupBuildContextEnv(t)
		t.Setenv("GCLUSTER_IMAGE_REPO", "")
		t.Setenv("USER", "")
		t.Setenv("USERNAME", "")

		ctxDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(ctxDir, "train.py"), []byte("print(1)\n"), 0644); err != nil {
			t.Fatal(err)
		}

		mockOrc := &mockOrchestrator{}
		oldFactory := gkeOrchestratorFactory
		t.Cleanup(func() { gkeOrchestratorFactory = oldFactory })
		gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

		_, err := executeCommand(JobCmd,
			"submit",
			"--skip-prereqs",
			"--cluster", "test-cluster",
			"--location", "us-central1-a",
			"--project", "test-project",
			"--name", "private-oci-build",
			"--base-image", "10.128.0.68:5000/jax:v5p",
			"--build-context", ctxDir,
			"--command", "python3 train.py",
			"--compute-type", "v5p-8",
		)
		if err != nil {
			t.Fatalf("expected private/local --base-image + --build-context to pass validation without GCLUSTER_IMAGE_REPO or USER, got: %v", err)
		}
		if mockOrc.SubmittedJob.WorkloadName != "private-oci-build" || mockOrc.SubmittedJob.BaseImage != "10.128.0.68:5000/jax:v5p" {
			t.Errorf("unexpected SubmittedJob: %+v", mockOrc.SubmittedJob)
		}
	})

	t.Run("explicit non-GAR GCLUSTER_IMAGE_REPO succeeds without USER env", func(t *testing.T) {
		setupBuildContextEnv(t)
		t.Setenv("GCLUSTER_IMAGE_REPO", "harbor.example.com/ml-team")
		t.Setenv("USER", "")
		t.Setenv("USERNAME", "")
		buildContext = t.TempDir()
		baseImage = "python:3.11-slim"

		if err := validateBuildContext(context.Background(), "", ""); err != nil {
			t.Fatalf("expected explicit non-GAR GCLUSTER_IMAGE_REPO without USER to pass, got: %v", err)
		}
	})

	t.Run("public and cloud base-images without project/location or GCLUSTER_IMAGE_REPO give OCI guidance", func(t *testing.T) {
		for _, pubBase := range []string{
			"python:3.11-slim",
			"cgr.dev/chainguard/python:latest",
			"registry.access.redhat.com/ubi9/python-311:latest",
			"mycorp.azurecr.io/ml/base:v1",
			"123456789012.dkr.ecr.us-east-1.amazonaws.com/ml/base:v1",
		} {
			setupBuildContextEnv(t)
			t.Setenv("GCLUSTER_IMAGE_REPO", "")
			buildContext = t.TempDir()
			baseImage = pubBase

			err := validateBuildContext(context.Background(), "", "")
			if err == nil || !strings.Contains(err.Error(), "<registry-host>/<repository>") {
				t.Fatalf("baseImage=%q: expected OCI GCLUSTER_IMAGE_REPO=<registry-host>/<repository> guidance, got: %v", pubBase, err)
			}
		}
	})

	t.Run("malformed GCLUSTER_IMAGE_REPO fails immediately in validateBuildContext", func(t *testing.T) {
		for _, badRepo := range []string{
			"harbor.local/ml-team:latest",
			"harbor.local/invalid!!repo",
			"harbor.local/a//b",
			"harbor.local/a/../b",
		} {
			setupBuildContextEnv(t)
			t.Setenv("GCLUSTER_IMAGE_REPO", badRepo)
			buildContext = t.TempDir()
			baseImage = "python:3.11-slim"

			if err := validateBuildContext(context.Background(), "", ""); err == nil {
				t.Fatalf("expected malformed GCLUSTER_IMAGE_REPO=%q to fail in validateBuildContext", badRepo)
			}
		}
	})
}
