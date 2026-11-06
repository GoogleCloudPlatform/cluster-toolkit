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

package gke

import (
	"errors"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"
)

type panicOnNamespaceKubeClient struct {
	KubeClient
	t *testing.T
}

func (p *panicOnNamespaceKubeClient) GetCurrentNamespace(_, _, _ string) (string, error) {
	p.t.Fatalf("GetCurrentNamespace must not be invoked for prebuilt --image jobs")
	return "", nil
}

func TestBuildContainerImage_NamespacePrecedenceAndPrebuiltHermeticity(t *testing.T) {
	t.Run("prebuilt ImageName never touches kubeconfig namespace resolution", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.kubeClient = &panicOnNamespaceKubeClient{t: t}

		got, err := orc.BuildContainerImage(orchestrator.JobDefinition{
			WorkloadName: "prebuilt-job",
			ImageName:    "10.128.0.68:5000/ml/jax:v5p",
		})
		if err != nil || got != "10.128.0.68:5000/ml/jax:v5p" {
			t.Fatalf("BuildContainerImage(prebuilt) = (%q, %v), want prebuilt URI", got, err)
		}
	})

	t.Run("job.GKENamespace takes precedence over g.namespace and kubeconfig on dry-run URI prediction", func(t *testing.T) {
		t.Setenv("GCLUSTER_IMAGE_REPO", "")
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.namespace = "stale-orch-ns"
		orc.kubeClient = &MockKubeClient{Namespace: "kubeconfig-ns"}

		got, err := orc.BuildContainerImage(orchestrator.JobDefinition{
			WorkloadName:   "train-v5p",
			GKENamespace:   "explicit-flag-ns",
			BaseImage:      "10.128.0.68:5000/ml/jax:v5p",
			DryRunManifest: "/tmp/dry-run.yaml",
		})
		if err != nil || !strings.HasPrefix(got, "10.128.0.68:5000/gcluster/explicit-flag-ns/train-v5p:") {
			t.Fatalf("BuildContainerImage(explicit GKENamespace) = (%q, %v)", got, err)
		}
	})

	t.Run("falls back to kubeconfig namespace then default when unset", func(t *testing.T) {
		t.Setenv("GCLUSTER_IMAGE_REPO", "")
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.kubeClient = &MockKubeClient{Namespace: "ctx-ns"}

		got, err := orc.BuildContainerImage(orchestrator.JobDefinition{
			WorkloadName:   "train-v5p",
			BaseImage:      "10.128.0.68:5000/ml/jax:v5p",
			DryRunManifest: "/tmp/dry-run.yaml",
		})
		if err != nil || !strings.HasPrefix(got, "10.128.0.68:5000/gcluster/ctx-ns/train-v5p:") {
			t.Fatalf("BuildContainerImage(ctx-ns) = (%q, %v)", got, err)
		}

		orcFallback := newTestGKEOrchestrator(NewMockExecutor(nil))
		orcFallback.kubeClient = &MockKubeClient{ExplicitEmpty: true, Err: errors.New("no kubeconfig")}
		got, err = orcFallback.BuildContainerImage(orchestrator.JobDefinition{
			WorkloadName:   "train-v5p",
			BaseImage:      "10.128.0.68:5000/ml/jax:v5p",
			DryRunManifest: "/tmp/dry-run.yaml",
		})
		if err != nil || !strings.HasPrefix(got, "10.128.0.68:5000/gcluster/default/train-v5p:") {
			t.Fatalf("BuildContainerImage(default fallback) = (%q, %v)", got, err)
		}
	})
}
