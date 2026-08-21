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
	"context"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCleanAndProcessManifests(t *testing.T) {
	orc := &GKEOrchestrator{}

	inputYAML := `
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  description: this should be removed
spec:
  containers:
  - name: main
    image: nginx
    description: this should also be removed
`

	cleaned, err := orc.cleanAndProcessManifests([]byte(inputYAML), nil)
	if err != nil {
		t.Fatalf("cleanAndProcessManifests failed: %v", err)
	}

	output := string(cleaned)

	if strings.Contains(output, "description:") {
		t.Errorf("expected descriptions to be removed, but got: %s", output)
	}
	if !strings.Contains(output, "test-pod") {
		t.Errorf("expected test-pod to be preserved, but got: %s", output)
	}
}

func TestValidateClusterState_TargetNamespaceValidation(t *testing.T) {
	mockExec := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 0}
		},
	}

	mockDyn := &mockDynamicClient{
		getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, name)
		},
	}

	orc := &GKEOrchestrator{
		executor:  mockExec,
		dynClient: mockDyn,
		kubeClient: &MockKubeClient{
			Namespace: "nonexistent-ns",
		},
	}

	job := &orchestrator.JobDefinition{
		ClusterName:     "test-cluster",
		ClusterLocation: "us-central1-a",
		ProjectID:       "test-project",
		GKENamespace:    "nonexistent-ns",
	}

	err := orc.ValidateClusterState(job)
	if err == nil {
		t.Fatal("expected ValidateClusterState to fail when namespace validation fails, got nil")
	}

	expectedErr := `target namespace "nonexistent-ns" does not exist`
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error to contain %q, got: %v", expectedErr, err)
	}
}

func TestReplaceDeprecatedRbacProxyImage(t *testing.T) {
	podSpec := map[interface{}]interface{}{
		"initContainers": []interface{}{
			map[interface{}]interface{}{
				"name":  "init-proxy",
				"image": "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1",
			},
		},
		"containers": []interface{}{
			map[interface{}]interface{}{
				"name":  "proxy",
				"image": "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1",
			},
			map[interface{}]interface{}{
				"name":  "manager",
				"image": "gcr.io/k8s-staging-jobset/jobset:v0.2.0",
			},
		},
	}

	replaceDeprecatedRbacProxyImage(podSpec)

	initContainers := podSpec["initContainers"].([]interface{})
	initProxy := initContainers[0].(map[interface{}]interface{})
	if got := initProxy["image"]; got != "quay.io/brancz/kube-rbac-proxy:v0.13.1" {
		t.Errorf("initContainers image = %v; want quay.io/brancz/kube-rbac-proxy:v0.13.1", got)
	}

	containers := podSpec["containers"].([]interface{})
	proxy := containers[0].(map[interface{}]interface{})
	if got := proxy["image"]; got != "quay.io/brancz/kube-rbac-proxy:v0.13.1" {
		t.Errorf("containers image = %v; want quay.io/brancz/kube-rbac-proxy:v0.13.1", got)
	}

	manager := containers[1].(map[interface{}]interface{})
	if got := manager["image"]; got != "gcr.io/k8s-staging-jobset/jobset:v0.2.0" {
		t.Errorf("manager image = %v; want gcr.io/k8s-staging-jobset/jobset:v0.2.0", got)
	}
}

func TestIsJobSetCRDInstalled_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{
				ExitCode: 1,
				Stderr:   "Error from server (Forbidden): customresourcedefinitions.apiextensions.k8s.io \"jobsets.jobset.x-k8s.io\" is forbidden",
			}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	installed, err := orc.isJobSetCRDInstalled()
	if err != nil {
		t.Fatalf("isJobSetCRDInstalled should succeed and return true on 403 Forbidden, got err: %v", err)
	}
	if !installed {
		t.Errorf("isJobSetCRDInstalled on 403 Forbidden = false; want true")
	}
}
