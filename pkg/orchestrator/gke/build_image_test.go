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

type recordingNamespaceKubeClient struct {
	KubeClient
	t           *testing.T
	forbidCalls bool
	ns          string
	err         error
	gotCluster  string
	gotLocation string
	gotProject  string
}

func (r *recordingNamespaceKubeClient) GetCurrentNamespace(cluster, location, project string) (string, error) {
	if r.forbidCalls {
		r.t.Fatalf("GetCurrentNamespace must not be invoked for prebuilt --image jobs")
	}
	r.gotCluster = cluster
	r.gotLocation = location
	r.gotProject = project
	return r.ns, r.err
}

func TestBuildContainerImage_NamespacePrecedenceAndPrebuiltHermeticity(t *testing.T) {
	t.Run("prebuilt ImageName never touches kubeconfig namespace resolution", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.kubeClient = &recordingNamespaceKubeClient{t: t, forbidCalls: true}

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
		orc.kubeClient = &recordingNamespaceKubeClient{t: t, forbidCalls: true}

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

	t.Run("delegates cluster/location/project to GetCurrentNamespace and propagates kubeconfig errors", func(t *testing.T) {
		t.Setenv("GCLUSTER_IMAGE_REPO", "")
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		spy := &recordingNamespaceKubeClient{t: t, ns: "ctx-ns"}
		orc.kubeClient = spy

		got, err := orc.BuildContainerImage(orchestrator.JobDefinition{
			WorkloadName:    "train-v5p",
			ClusterName:     "clus-a",
			ClusterLocation: "us-central1-a",
			ProjectID:       "proj-a",
			BaseImage:       "10.128.0.68:5000/ml/jax:v5p",
			DryRunManifest:  "/tmp/dry-run.yaml",
		})
		if err != nil || !strings.HasPrefix(got, "10.128.0.68:5000/gcluster/ctx-ns/train-v5p:") {
			t.Fatalf("BuildContainerImage(ctx-ns) = (%q, %v)", got, err)
		}
		if spy.gotCluster != "clus-a" || spy.gotLocation != "us-central1-a" || spy.gotProject != "proj-a" {
			t.Fatalf("GetCurrentNamespace args = (%q, %q, %q), want (clus-a, us-central1-a, proj-a)", spy.gotCluster, spy.gotLocation, spy.gotProject)
		}

		orcBroken := newTestGKEOrchestrator(NewMockExecutor(nil))
		orcBroken.kubeClient = &recordingNamespaceKubeClient{t: t, err: errors.New("broken kubeconfig")}
		_, err = orcBroken.BuildContainerImage(orchestrator.JobDefinition{
			WorkloadName:   "train-v5p",
			BaseImage:      "10.128.0.68:5000/ml/jax:v5p",
			DryRunManifest: "/tmp/dry-run.yaml",
		})
		if err == nil || !strings.Contains(err.Error(), "broken kubeconfig") {
			t.Fatalf("expected broken kubeconfig error to propagate instead of silently falling back to default, got: %v", err)
		}
	})
}
