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
	"fmt"
	"strings"
	"testing"
	"time"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
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

	var validated []string
	mockDyn := &mockDynamicClient{
		getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
			validated = append(validated, name)
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, name)
		},
	}

	// The kubeconfig namespace deliberately differs from --gke-namespace so the
	// test proves the seeded namespace (not the kubeconfig one) is validated.
	kube := &countingKubeClient{MockKubeClient: MockKubeClient{Namespace: "kube-ctx-ns"}}
	orc := &GKEOrchestrator{
		executor:   mockExec,
		dynClient:  mockDyn,
		kubeClient: kube,
	}

	job := &orchestrator.JobDefinition{
		ClusterName:     "test-cluster",
		ClusterLocation: "us-central1-a",
		ProjectID:       "test-project",
		GKENamespace:    "nonexistent-ns",
	}
	orc.namespace = job.GKENamespace // as done at the top of SubmitJob

	err := orc.ValidateClusterState(job)
	if err == nil {
		t.Fatal("expected ValidateClusterState to fail when namespace validation fails, got nil")
	}

	expectedErr := `target namespace "nonexistent-ns" does not exist on GKE cluster "test-cluster"`
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error to contain %q, got: %v", expectedErr, err)
	}
	if len(validated) != 1 || validated[0] != "nonexistent-ns" {
		t.Errorf("validated namespaces = %v, want [nonexistent-ns]", validated)
	}
	if kube.calls != 0 {
		t.Errorf("kubeconfig namespace lookup should be skipped when --gke-namespace is set, got %d calls", kube.calls)
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

func TestInitialize_LocationFallback(t *testing.T) {
	setupMockMachineConfig(t)

	mockDescribeOutput := `{
		"locations": ["us-central1-a", "us-central1-b"],
		"nodePools": [],
		"autoscaling": {}
	}`

	mockResponses := map[string][]shell.CommandResult{
		"gcloud container clusters describe my-cluster --location us-central1-a --project my-project --format=json": {
			{
				ExitCode: 1,
				Stderr:   "Resource my-cluster was not found in us-central1-a",
			},
		},
		"gcloud container clusters describe my-cluster --location us-central1 --project my-project --format=json": {
			{
				ExitCode: 0,
				Stdout:   mockDescribeOutput,
			},
		},
	}

	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	job := &orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterName:     "my-cluster",
		ClusterLocation: "us-central1-a",
	}

	loc, err := orc.Initialize(job.ClusterName, job.ClusterLocation, job.ProjectID)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	job.ClusterLocation = loc

	err = orc.populateClusterMetadata(job)
	if err != nil {
		t.Fatalf("populateClusterMetadata failed: %v", err)
	}

	if job.ClusterLocation != "us-central1" {
		t.Errorf("Expected job.ClusterLocation to fall back to 'us-central1', got %q", job.ClusterLocation)
	}
}

// countingKubeClient wraps MockKubeClient to count GetCurrentNamespace calls.
type countingKubeClient struct {
	MockKubeClient
	calls int
}

func (c *countingKubeClient) GetCurrentNamespace(clusterName, location, projectID string) (string, error) {
	c.calls++
	return c.MockKubeClient.GetCurrentNamespace(clusterName, location, projectID)
}

func TestGetCurrentNamespace(t *testing.T) {
	tests := []struct {
		name          string
		cached        string
		kubeClient    *countingKubeClient
		want          string
		wantErr       bool
		wantKubeCalls int
		wantCached    string
	}{
		{
			name:          "cached namespace (from --gke-namespace) short-circuits kubeconfig lookup",
			cached:        "explicit-ns",
			kubeClient:    &countingKubeClient{MockKubeClient: MockKubeClient{Namespace: "kube-ns"}},
			want:          "explicit-ns",
			wantKubeCalls: 0,
			wantCached:    "explicit-ns",
		},
		{
			name:          "falls back to kubeconfig namespace and caches it",
			kubeClient:    &countingKubeClient{MockKubeClient: MockKubeClient{Namespace: "current-ns"}},
			want:          "current-ns",
			wantKubeCalls: 1,
			wantCached:    "current-ns",
		},
		{
			name:          "kubeconfig lookup failure is propagated and not cached",
			kubeClient:    &countingKubeClient{MockKubeClient: MockKubeClient{Err: fmt.Errorf("kubeclient error")}},
			wantErr:       true,
			wantKubeCalls: 1,
			wantCached:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GKEOrchestrator{kubeClient: tt.kubeClient, namespace: tt.cached}
			got, err := g.getCurrentNamespace("cluster", "us-central1", "project")
			if (err != nil) != tt.wantErr {
				t.Fatalf("getCurrentNamespace() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("getCurrentNamespace() = %q, want %q", got, tt.want)
			}
			if tt.kubeClient.calls != tt.wantKubeCalls {
				t.Errorf("GetCurrentNamespace calls = %d, want %d", tt.kubeClient.calls, tt.wantKubeCalls)
			}
			if g.namespace != tt.wantCached {
				t.Errorf("cached g.namespace = %q, want %q", g.namespace, tt.wantCached)
			}

			// A second call must be served from cache without another lookup.
			if !tt.wantErr {
				if _, err := g.getCurrentNamespace("cluster", "us-central1", "project"); err != nil {
					t.Fatalf("second getCurrentNamespace() error = %v", err)
				}
				if tt.kubeClient.calls != tt.wantKubeCalls {
					t.Errorf("second call triggered kubeconfig lookup: calls = %d, want %d", tt.kubeClient.calls, tt.wantKubeCalls)
				}
			}
		})
	}
}

func TestValidateNamespaceExists(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		kubeClient    KubeClient
		dynClient     dynamic.Interface
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:          "kubeconfig namespace lookup failure is propagated",
			namespace:     "",
			kubeClient:    &MockKubeClient{Err: fmt.Errorf("failed to initialize Kubernetes client")},
			dynClient:     nil,
			wantErr:       true,
			wantErrSubstr: "failed to initialize Kubernetes client",
		},
		{
			name:       "namespace exists",
			namespace:  "exists",
			kubeClient: &MockKubeClient{Namespace: "exists"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return &unstructured.Unstructured{Object: map[string]interface{}{"kind": "Namespace"}}, nil
				},
			},
			wantErr: false,
		},
		{
			name:       "explicit --gke-namespace override wins over kubeconfig namespace",
			namespace:  "custom-job-ns",
			kubeClient: &MockKubeClient{Namespace: "default-kube-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					if name != "custom-job-ns" {
						return nil, fmt.Errorf("expected Get for custom-job-ns, got %s", name)
					}
					return &unstructured.Unstructured{Object: map[string]interface{}{"kind": "Namespace"}}, nil
				},
			},
			wantErr: false,
		},
		{
			name:       "falls back to kubeconfig namespace when --gke-namespace unset",
			namespace:  "",
			kubeClient: &MockKubeClient{Namespace: "kube-ctx-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					if name != "kube-ctx-ns" {
						return nil, fmt.Errorf("expected Get for kube-ctx-ns, got %s", name)
					}
					return &unstructured.Unstructured{Object: map[string]interface{}{"kind": "Namespace"}}, nil
				},
			},
			wantErr: false,
		},
		{
			name:       "namespace does not exist",
			namespace:  "nonexistent",
			kubeClient: &MockKubeClient{Namespace: "nonexistent"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, name)
				},
			},
			wantErr:       true,
			wantErrSubstr: `target namespace "nonexistent" does not exist on GKE cluster "test-cluster"`,
		},
		{
			name:          "empty namespace",
			namespace:     "",
			kubeClient:    &MockKubeClient{ExplicitEmpty: true},
			dynClient:     &mockDynamicClient{},
			wantErr:       true,
			wantErrSubstr: `target namespace cannot be empty for GKE cluster "test-cluster". Please pass --gke-namespace, or set a default namespace in your kubeconfig context`,
		},
		{
			name:       "403 forbidden (RBAC restricted user proceeds with warning)",
			namespace:  "restricted-ns",
			kubeClient: &MockKubeClient{Namespace: "restricted-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, name, fmt.Errorf("user cannot get resource"))
				},
			},
			wantErr: false,
		},
		{
			name:       "403 forbidden string error (RBAC restricted user proceeds with warning)",
			namespace:  "restricted-string-ns",
			kubeClient: &MockKubeClient{Namespace: "restricted-string-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return nil, fmt.Errorf("Error from server (Forbidden): namespaces %q is forbidden", name)
				},
			},
			wantErr: false,
		},
		{
			name:       "other API error is wrapped",
			namespace:  "some-ns",
			kubeClient: &MockKubeClient{Namespace: "some-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return nil, fmt.Errorf("connection refused")
				},
			},
			wantErr:       true,
			wantErrSubstr: `failed to verify existence of namespace "some-ns" on cluster "test-cluster": connection refused`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mirrors SubmitJob, which seeds g.namespace from job.GKENamespace.
			orch := &GKEOrchestrator{
				kubeClient: tt.kubeClient,
				dynClient:  tt.dynClient,
				namespace:  tt.namespace,
			}

			err := orch.validateTargetNamespaceExists("test-cluster", "us-central1-a", "test-project")

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected an error, but got nil")
				} else if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrSubstr, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})
	}
}

func TestValidateMTCConfig(t *testing.T) {
	tests := []struct {
		name          string
		job           *orchestrator.JobDefinition
		clusterDesc   gkeCluster
		dynClient     dynamic.Interface
		kubeClient    KubeClient
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name: "MTC Disabled - Pass",
			job:  &orchestrator.JobDefinition{GKEMTCEnabled: false},
		},
		{
			name:          "MTC Enabled without HighScaleCheckpointing Addon - Fail",
			job:           &orchestrator.JobDefinition{GKEMTCEnabled: true, GKEMTCRamdiskDirectory: "/tmp/ramdisk"},
			wantErr:       true,
			wantErrSubstr: "HighScaleCheckpointing addon",
		},
		{
			name: "MTC Enabled with DryRunManifest - Pass without k8s client",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				DryRunManifest:         "manifest.yaml",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
		},
		{
			name: "MTC Enabled with CheckpointConfiguration CR present - Pass",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "custom-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return &unstructured.UnstructuredList{
						Items: []unstructured.Unstructured{
							{Object: map[string]interface{}{"kind": "CheckpointConfiguration"}},
						},
					}, nil
				},
			},
		},
		{
			name: "MTC Enabled with CheckpointConfiguration CR missing - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "custom-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
				},
			},
			wantErr:       true,
			wantErrSubstr: "requires a CheckpointConfiguration resource",
		},
		{
			name: "MTC Enabled with 403 Forbidden - Pass with warning",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "restricted-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "checkpointconfigurations"}, "test", fmt.Errorf("forbidden"))
				},
			},
		},
		{
			name: "MTC Enabled with CheckpointConfiguration CRD Unregistered - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "custom-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "checkpointconfigurations"}, "")
				},
			},
			wantErr:       true,
			wantErrSubstr: "is not registered on the cluster",
		},
		{
			name: "MTC Enabled with Empty Ramdisk Directory - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "",
			},
			wantErr:       true,
			wantErrSubstr: "ramdisk directory path (--gke-mtc-ramdisk-dir) cannot be empty",
		},
		{
			name: "MTC Enabled with generic k8s client error - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "custom-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, fmt.Errorf("internal server error")
				},
			},
			wantErr:       true,
			wantErrSubstr: "failed to verify CheckpointConfiguration resource",
		},
		{
			name: "MTC Enabled with Invalid Ramdisk Path (Relative) - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "relative/path",
			},
			wantErr:       true,
			wantErrSubstr: "--gke-mtc-ramdisk-dir must be an absolute path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{
				clusterDesc: tt.clusterDesc,
				dynClient:   tt.dynClient,
				kubeClient:  tt.kubeClient,
			}
			err := orc.validateMTCConfig(tt.job)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected an error, but got nil")
				} else if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrSubstr, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})
	}
}

func TestGetNodeServiceAccount(t *testing.T) {
	tests := []struct {
		name        string
		clusterDesc gkeCluster
		wantSA      string
	}{
		{
			name: "No node pools",
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{},
			},
			wantSA: "",
		},
		{
			name: "Default service account ignored",
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{
					{Config: gkeNodePoolConfig{ServiceAccount: "default"}},
				},
			},
			wantSA: "",
		},
		{
			name: "Custom node pool service account found",
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{
					{Config: gkeNodePoolConfig{ServiceAccount: "default"}},
					{Config: gkeNodePoolConfig{ServiceAccount: "custom-gsa@project.iam.gserviceaccount.com"}},
				},
			},
			wantSA: "custom-gsa@project.iam.gserviceaccount.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{clusterDesc: tt.clusterDesc}
			got := orc.getNodeServiceAccount()
			if got != tt.wantSA {
				t.Errorf("getNodeServiceAccount() = %q, want %q", got, tt.wantSA)
			}
		})
	}
}

func TestEnsureMTCWorkloadIdentity_Basic(t *testing.T) {
	const nodeSA = "test-node-sa@test-proj.iam.gserviceaccount.com"
	clusterWithSA := gkeCluster{
		NodePools: []gkeJobNodePool{
			{Config: gkeNodePoolConfig{ServiceAccount: nodeSA}},
		},
	}

	t.Run("No custom node SA - No op", func(t *testing.T) {
		orc := &GKEOrchestrator{
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{
					{Config: gkeNodePoolConfig{ServiceAccount: "default"}},
				},
			},
		}
		err := orc.ensureMTCWorkloadIdentity()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("Annotation already correct - No update or restart", func(t *testing.T) {
		updated := false
		deleted := false
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"iam.gke.io/gcp-service-account": nodeSA,
							},
						},
					},
				}, nil
			},
			updateFunc: func(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
				updated = true
				return obj, nil
			},
			deleteFunc: func(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
				deleted = true
				return nil
			},
		}

		orc := &GKEOrchestrator{
			clusterDesc: clusterWithSA,
			dynClient:   dynClient,
		}
		err := orc.ensureMTCWorkloadIdentity()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if updated || deleted {
			t.Errorf("expected no update or delete when annotation is correct, got updated=%v, deleted=%v", updated, deleted)
		}
	})
}

func TestEnsureMTCWorkloadIdentity_Restart(t *testing.T) {
	const nodeSA = "test-node-sa@test-proj.iam.gserviceaccount.com"
	clusterWithSA := gkeCluster{
		NodePools: []gkeJobNodePool{
			{Config: gkeNodePoolConfig{ServiceAccount: nodeSA}},
		},
	}

	oldInterval := daemonSetPollInterval
	daemonSetPollInterval = 1 * time.Millisecond
	defer func() { daemonSetPollInterval = oldInterval }()

	t.Run("Annotation missing - Updates SA and restarts DaemonSet", func(t *testing.T) {
		updated := false
		patchedDS := false
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				if name == "multitier-driver" {
					return &unstructured.Unstructured{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"name":       "multitier-driver",
								"generation": int64(1),
							},
							"status": map[string]interface{}{
								"observedGeneration":     int64(1),
								"desiredNumberScheduled": int64(1),
								"numberReady":            int64(1),
								"updatedNumberScheduled": int64(1),
							},
						},
					}, nil
				}
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{},
						},
					},
				}, nil
			},
			updateFunc: func(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
				updated = true
				return obj, nil
			},
			patchFunc: func(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
				if name == "multitier-driver" {
					patchedDS = true
				}
				return &unstructured.Unstructured{}, nil
			},
		}

		orc := &GKEOrchestrator{
			clusterDesc: clusterWithSA,
			dynClient:   dynClient,
		}
		err := orc.ensureMTCWorkloadIdentity()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !updated {
			t.Errorf("expected SA to be updated, but was not")
		}
		if !patchedDS {
			t.Errorf("expected DaemonSet multitier-driver to be patched for restart, but was not")
		}
	})

	t.Run("Annotation missing - Updates SA and falls back to restarting driver pods when DaemonSet patch fails", func(t *testing.T) {
		updated := false
		deletedPods := []string{}
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				if name == "multitier-driver" {
					return &unstructured.Unstructured{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"name":       "multitier-driver",
								"generation": int64(1),
							},
							"status": map[string]interface{}{
								"observedGeneration":     int64(1),
								"desiredNumberScheduled": int64(1),
								"numberReady":            int64(1),
								"updatedNumberScheduled": int64(1),
							},
						},
					}, nil
				}
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{},
						},
					},
				}, nil
			},
			updateFunc: func(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
				updated = true
				return obj, nil
			},
			patchFunc: func(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, fmt.Errorf("daemonset not found")
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				return &unstructured.UnstructuredList{
					Items: []unstructured.Unstructured{
						{
							Object: map[string]interface{}{
								"metadata": map[string]interface{}{
									"name": "multitier-driver-abc12",
								},
							},
						},
						{
							Object: map[string]interface{}{
								"metadata": map[string]interface{}{
									"name": "other-pod-xyz",
								},
							},
						},
					},
				}, nil
			},
			deleteFunc: func(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
				deletedPods = append(deletedPods, name)
				return nil
			},
		}

		orc := &GKEOrchestrator{
			clusterDesc: clusterWithSA,
			dynClient:   dynClient,
		}
		err := orc.ensureMTCWorkloadIdentity()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !updated {
			t.Errorf("expected SA to be updated, but was not")
		}
		if len(deletedPods) != 1 || deletedPods[0] != "multitier-driver-abc12" {
			t.Errorf("expected driver pod multitier-driver-abc12 to be deleted, got %v", deletedPods)
		}
	})
}

func TestEnsureMTCWorkloadIdentity_RBAC(t *testing.T) {
	const nodeSA = "test-node-sa@test-proj.iam.gserviceaccount.com"
	clusterWithSA := gkeCluster{
		NodePools: []gkeJobNodePool{
			{Config: gkeNodePoolConfig{ServiceAccount: nodeSA}},
		},
	}

	oldInterval := daemonSetPollInterval
	daemonSetPollInterval = 1 * time.Millisecond
	defer func() { daemonSetPollInterval = oldInterval }()

	t.Run("Get SA fails - Self-healing logs warning and continues without failing job", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "serviceaccounts"}, "gke-checkpointing-multitier-node")
			},
		}
		orc := &GKEOrchestrator{
			clusterDesc: clusterWithSA,
			dynClient:   dynClient,
		}
		err := orc.ensureMTCWorkloadIdentity()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("403 Forbidden on Get SA - Continues gracefully", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, "gke-checkpointing-multitier-node", fmt.Errorf("forbidden"))
			},
		}
		orc := &GKEOrchestrator{
			clusterDesc: clusterWithSA,
			dynClient:   dynClient,
		}
		err := orc.ensureMTCWorkloadIdentity()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("403 Forbidden on Update SA - Continues gracefully", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{},
						},
					},
				}, nil
			},
			updateFunc: func(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, "gke-checkpointing-multitier-node", fmt.Errorf("forbidden"))
			},
		}
		orc := &GKEOrchestrator{
			clusterDesc: clusterWithSA,
			dynClient:   dynClient,
		}
		err := orc.ensureMTCWorkloadIdentity()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("403 Forbidden on List driver pods - Continues gracefully", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				if name == "multitier-driver" {
					return &unstructured.Unstructured{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"name":       "multitier-driver",
								"generation": int64(1),
							},
							"status": map[string]interface{}{
								"observedGeneration":     int64(1),
								"desiredNumberScheduled": int64(1),
								"numberReady":            int64(1),
								"updatedNumberScheduled": int64(1),
							},
						},
					}, nil
				}
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{},
						},
					},
				}, nil
			},
			updateFunc: func(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return obj, nil
			},
			patchFunc: func(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, fmt.Errorf("patch failed")
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", fmt.Errorf("forbidden"))
			},
		}
		orc := &GKEOrchestrator{
			clusterDesc: clusterWithSA,
			dynClient:   dynClient,
		}
		err := orc.ensureMTCWorkloadIdentity()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}

func TestWaitForDaemonSetRollout(t *testing.T) {
	oldInterval := daemonSetPollInterval
	daemonSetPollInterval = 1 * time.Millisecond
	defer func() { daemonSetPollInterval = oldInterval }()

	t.Run("DaemonSet ready immediately", func(t *testing.T) {
		calls := 0
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				calls++
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"generation": int64(2)},
						"status": map[string]interface{}{
							"observedGeneration":     int64(2),
							"desiredNumberScheduled": int64(3),
							"numberReady":            int64(3),
							"updatedNumberScheduled": int64(3),
						},
					},
				}, nil
			},
		}
		waitForDaemonSetRollout(context.Background(), dynClient, "gke-managed-checkpointing", "multitier-driver-uuid")
		if calls != 1 {
			t.Errorf("expected 1 call for immediate ready, got %d", calls)
		}
	})

	t.Run("DaemonSet rollout completes after polling", func(t *testing.T) {
		calls := 0
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				calls++
				if calls < 2 {
					return &unstructured.Unstructured{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{"generation": int64(2)},
							"status": map[string]interface{}{
								"observedGeneration":     int64(2),
								"desiredNumberScheduled": int64(3),
								"numberReady":            int64(1),
								"updatedNumberScheduled": int64(1),
							},
						},
					}, nil
				}
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"generation": int64(2)},
						"status": map[string]interface{}{
							"observedGeneration":     int64(2),
							"desiredNumberScheduled": int64(3),
							"numberReady":            int64(3),
							"updatedNumberScheduled": int64(3),
						},
					},
				}, nil
			},
		}
		waitForDaemonSetRollout(context.Background(), dynClient, "gke-managed-checkpointing", "multitier-driver-uuid")
		if calls < 2 {
			t.Errorf("expected at least 2 calls for polling, got %d", calls)
		}
	})

	t.Run("403 Forbidden returns gracefully", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "daemonsets"}, name, fmt.Errorf("forbidden"))
			},
		}
		waitForDaemonSetRollout(context.Background(), dynClient, "gke-managed-checkpointing", "multitier-driver-uuid")
	})

	t.Run("Context canceled returns gracefully", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{}, nil
			},
		}
		waitForDaemonSetRollout(ctx, dynClient, "gke-managed-checkpointing", "multitier-driver-uuid")
	})

	t.Run("Transient error during polling retries and succeeds", func(t *testing.T) {
		calls := 0
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				calls++
				if calls == 1 {
					return nil, fmt.Errorf("transient network failure")
				}
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"generation": int64(2)},
						"status": map[string]interface{}{
							"observedGeneration":     int64(2),
							"desiredNumberScheduled": int64(3),
							"numberReady":            int64(3),
							"updatedNumberScheduled": int64(3),
						},
					},
				}, nil
			},
		}
		waitForDaemonSetRollout(context.Background(), dynClient, "gke-managed-checkpointing", "multitier-driver-uuid")
		if calls < 2 {
			t.Errorf("expected at least 2 polling calls after transient retry, got %d", calls)
		}
	})
}

func TestIsDaemonSetRolloutComplete(t *testing.T) {
	tests := []struct {
		name      string
		gen       int64
		obsGen    int64
		desired   int64
		ready     int64
		updated   int64
		wantReady bool
	}{
		{
			name:      "zero desired scheduled pods (uninitialized)",
			gen:       1,
			obsGen:    1,
			desired:   0,
			ready:     0,
			updated:   0,
			wantReady: false,
		},
		{
			name:      "observedGeneration behind spec generation",
			gen:       2,
			obsGen:    1,
			desired:   3,
			ready:     3,
			updated:   3,
			wantReady: false,
		},
		{
			name:      "ready pods less than desired",
			gen:       2,
			obsGen:    2,
			desired:   3,
			ready:     2,
			updated:   3,
			wantReady: false,
		},
		{
			name:      "updated pods less than desired",
			gen:       2,
			obsGen:    2,
			desired:   3,
			ready:     3,
			updated:   2,
			wantReady: false,
		},
		{
			name:      "all pods ready and updated",
			gen:       2,
			obsGen:    2,
			desired:   3,
			ready:     3,
			updated:   3,
			wantReady: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"generation": tt.gen,
					},
					"status": map[string]interface{}{
						"observedGeneration":     tt.obsGen,
						"desiredNumberScheduled": tt.desired,
						"numberReady":            tt.ready,
						"updatedNumberScheduled": tt.updated,
					},
				},
			}
			got := isDaemonSetRolloutComplete(dsObj)
			if got != tt.wantReady {
				t.Errorf("isDaemonSetRolloutComplete() = %v, want %v", got, tt.wantReady)
			}
		})
	}

	t.Run("nil dsObj returns false", func(t *testing.T) {
		if isDaemonSetRolloutComplete(nil) {
			t.Errorf("isDaemonSetRolloutComplete(nil) = true, want false")
		}
	})

	t.Run("nil dsObj.Object returns false", func(t *testing.T) {
		if isDaemonSetRolloutComplete(&unstructured.Unstructured{}) {
			t.Errorf("isDaemonSetRolloutComplete(empty) = true, want false")
		}
	})
}

func TestIsForbiddenError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Nil error returns false",
			err:  nil,
			want: false,
		},
		{
			name: "Typed 403 Forbidden returns true",
			err:  apierrors.NewForbidden(schema.GroupResource{Resource: "services"}, "test-svc", fmt.Errorf("forbidden")),
			want: true,
		},
		{
			name: "Generic error with forbidden substring returns true",
			err:  fmt.Errorf("Error from server (Forbidden): request forbidden"),
			want: true,
		},
		{
			name: "Other error returns false",
			err:  fmt.Errorf("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isForbiddenError(tt.err); got != tt.want {
				t.Errorf("isForbiddenError(%v) = %v; want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestVerifyCheckpointConfigurationCR(t *testing.T) {
	const docRemediation = "Please follow the documentation."

	tests := []struct {
		name          string
		dynClient     dynamic.Interface
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name: "Success - CR exists",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return &unstructured.UnstructuredList{
						Items: []unstructured.Unstructured{
							{
								Object: map[string]interface{}{
									"metadata": map[string]interface{}{"name": "default"},
								},
							},
						},
					}, nil
				},
			},
			wantErr: false,
		},
		{
			name: "CRD Not Found - returns error",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "checkpointconfigurations"}, "")
				},
			},
			wantErr:       true,
			wantErrSubstr: "the CheckpointConfiguration CustomResourceDefinition (CRD) is not registered on the cluster",
		},
		{
			name: "403 Forbidden typed error - proceeds without error",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "checkpointconfigurations"}, "", fmt.Errorf("user cannot list resource"))
				},
			},
			wantErr: false,
		},
		{
			name: "403 Forbidden string error - proceeds without error",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, fmt.Errorf("Error from server (Forbidden): checkpointconfigurations.checkpointing.gke.io is forbidden")
				},
			},
			wantErr: false,
		},
		{
			name: "0 CR items - returns error",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
				},
			},
			wantErr:       true,
			wantErrSubstr: "Multi-Tier Checkpointing (MTC) requires a CheckpointConfiguration resource to be deployed on the cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{
				dynClient: tt.dynClient,
			}
			err := orc.verifyCheckpointConfigurationCR(docRemediation)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected an error, but got nil")
				} else if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrSubstr, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})
	}
}

func TestGetMTCDaemonSet(t *testing.T) {
	t.Run("Static name multitier-driver", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				if name == "multitier-driver" {
					return &unstructured.Unstructured{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"name": "multitier-driver",
							},
						},
					}, nil
				}
				return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name)
			},
		}
		ds, err := getMTCDaemonSet(context.Background(), dynClient, "gke-managed-checkpointing")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if ds.GetName() != "multitier-driver" {
			t.Errorf("expected DaemonSet name multitier-driver, got %s", ds.GetName())
		}
	})

	t.Run("Dynamic name multitier-driver-uuid discovered via list", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name)
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				return &unstructured.UnstructuredList{
					Items: []unstructured.Unstructured{
						{
							Object: map[string]interface{}{
								"metadata": map[string]interface{}{
									"name": "multitier-driver-fd6c6ab5-2191-47e6-8e95-11b3d9ac7cb6",
									"labels": map[string]interface{}{
										"k8s-app": "high-scale-checkpointing",
									},
								},
							},
						},
					},
				}, nil
			},
		}
		ds, err := getMTCDaemonSet(context.Background(), dynClient, "gke-managed-checkpointing")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		expectedName := "multitier-driver-fd6c6ab5-2191-47e6-8e95-11b3d9ac7cb6"
		if ds.GetName() != expectedName {
			t.Errorf("expected DaemonSet name %s, got %s", expectedName, ds.GetName())
		}
	})

	t.Run("403 Forbidden on Get returns error without listing", func(t *testing.T) {
		listCalled := false
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name, fmt.Errorf("forbidden"))
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				listCalled = true
				return nil, nil
			},
		}
		_, err := getMTCDaemonSet(context.Background(), dynClient, "gke-managed-checkpointing")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !isForbiddenError(err) {
			t.Errorf("expected forbidden error, got %v", err)
		}
		if listCalled {
			t.Errorf("expected List not to be called when Get returns 403 Forbidden")
		}
	})
}

func TestGetMTCDaemonSet_EdgeCases(t *testing.T) {
	t.Run("404 NotFound when Get fails and List returns no matching DaemonSets", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name)
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				return &unstructured.UnstructuredList{
					Items: []unstructured.Unstructured{
						{
							Object: map[string]interface{}{
								"metadata": map[string]interface{}{
									"name":   "kube-proxy",
									"labels": map[string]interface{}{"k8s-app": "kube-proxy"},
								},
							},
						},
					},
				}, nil
			},
		}
		ds, err := getMTCDaemonSet(context.Background(), dynClient, "gke-managed-checkpointing")
		if err == nil {
			t.Fatalf("expected NotFound error, got nil")
		}
		if !apierrors.IsNotFound(err) {
			t.Errorf("expected IsNotFound(err) == true, got %v", err)
		}
		if ds != nil {
			t.Errorf("expected nil ds, got %v", ds)
		}
	})

	t.Run("403 Forbidden on List propagates error", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name)
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				return nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, "", fmt.Errorf("forbidden"))
			},
		}
		_, err := getMTCDaemonSet(context.Background(), dynClient, "gke-managed-checkpointing")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !isForbiddenError(err) {
			t.Errorf("expected forbidden error, got %v", err)
		}
	})

	t.Run("Context canceled on Get returns error without calling List", func(t *testing.T) {
		listCalled := false
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, context.Canceled
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				listCalled = true
				return &unstructured.UnstructuredList{}, nil
			},
		}
		_, err := getMTCDaemonSet(context.Background(), dynClient, "gke-managed-checkpointing")
		if err == nil {
			t.Fatalf("expected context.Canceled error, got nil")
		}
		if listCalled {
			t.Errorf("expected List NOT to be called when Get returns context.Canceled")
		}
	})

	t.Run("Transient network error on Get returns error without calling List", func(t *testing.T) {
		listCalled := false
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, fmt.Errorf("connection refused")
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				listCalled = true
				return &unstructured.UnstructuredList{}, nil
			},
		}
		_, err := getMTCDaemonSet(context.Background(), dynClient, "gke-managed-checkpointing")
		if err == nil {
			t.Fatalf("expected network error, got nil")
		}
		if listCalled {
			t.Errorf("expected List NOT to be called when Get returns network error")
		}
	})

	t.Run("Filters unrelated DaemonSets and matches by label", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name)
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				return &unstructured.UnstructuredList{
					Items: []unstructured.Unstructured{
						{
							Object: map[string]interface{}{
								"metadata": map[string]interface{}{
									"name": "unrelated-daemonset",
								},
							},
						},
						{
							Object: map[string]interface{}{
								"metadata": map[string]interface{}{
									"name": "custom-checkpoint-driver",
									"labels": map[string]interface{}{
										"k8s-app": "high-scale-checkpointing",
									},
								},
							},
						},
					},
				}, nil
			},
		}
		ds, err := getMTCDaemonSet(context.Background(), dynClient, "gke-managed-checkpointing")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if ds.GetName() != "custom-checkpoint-driver" {
			t.Errorf("expected custom-checkpoint-driver, got %s", ds.GetName())
		}
	})
}

func TestRestartMTCDriverPods_DynamicName(t *testing.T) {
	oldInterval := daemonSetPollInterval
	daemonSetPollInterval = 1 * time.Millisecond
	defer func() { daemonSetPollInterval = oldInterval }()

	patchedName := ""
	dynClient := &mockDynamicClient{
		getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
			if name == "multitier-driver-8f2c7adf-1b1e-47e4-ba2e-104ffa2c846b" {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":       "multitier-driver-8f2c7adf-1b1e-47e4-ba2e-104ffa2c846b",
							"generation": int64(1),
						},
						"status": map[string]interface{}{
							"observedGeneration":     int64(1),
							"desiredNumberScheduled": int64(4),
							"numberReady":            int64(4),
							"updatedNumberScheduled": int64(4),
						},
					},
				}, nil
			}
			return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name)
		},
		listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"name": "multitier-driver-8f2c7adf-1b1e-47e4-ba2e-104ffa2c846b",
								"labels": map[string]interface{}{
									"k8s-app": "high-scale-checkpointing",
								},
							},
						},
					},
				},
			}, nil
		},
		patchFunc: func(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
			patchedName = name
			return &unstructured.Unstructured{}, nil
		},
	}

	restartMTCDriverPods(context.Background(), dynClient, "gke-managed-checkpointing")
	expectedName := "multitier-driver-8f2c7adf-1b1e-47e4-ba2e-104ffa2c846b"
	if patchedName != expectedName {
		t.Errorf("expected patched DaemonSet name %s, got %s", expectedName, patchedName)
	}
}

func TestRestartMTCDriverPods_ForbiddenAndNotFound(t *testing.T) {
	oldInterval := daemonSetPollInterval
	daemonSetPollInterval = 1 * time.Millisecond
	defer func() { daemonSetPollInterval = oldInterval }()

	t.Run("403 Forbidden on Patch skips pod deletion fallback", func(t *testing.T) {
		deleteCalled := false
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"name": "multitier-driver"},
					},
				}, nil
			},
			patchFunc: func(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name, fmt.Errorf("forbidden"))
			},
			deleteFunc: func(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
				deleteCalled = true
				return nil
			},
		}

		restartMTCDriverPods(context.Background(), dynClient, "gke-managed-checkpointing")
		if deleteCalled {
			t.Errorf("expected delete fallback NOT to be called when patch returns 403 Forbidden")
		}
	})

	t.Run("404 NotFound on getMTCDaemonSet skips pod deletion fallback", func(t *testing.T) {
		deleteCalled := false
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name)
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
			},
			deleteFunc: func(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
				deleteCalled = true
				return nil
			},
		}

		restartMTCDriverPods(context.Background(), dynClient, "gke-managed-checkpointing")
		if deleteCalled {
			t.Errorf("expected delete fallback NOT to be called when DaemonSet is not found")
		}
	})

	t.Run("Unexpected error on getMTCDaemonSet skips pod deletion fallback", func(t *testing.T) {
		deleteCalled := false
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, fmt.Errorf("connection timeout")
			},
			deleteFunc: func(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
				deleteCalled = true
				return nil
			},
		}

		restartMTCDriverPods(context.Background(), dynClient, "gke-managed-checkpointing")
		if deleteCalled {
			t.Errorf("expected delete fallback NOT to be called when getMTCDaemonSet returns unexpected error")
		}
	})

	t.Run("Non-forbidden error on Patch triggers pod deletion fallback", func(t *testing.T) {
		deleteCalled := false
		dynClient := &mockDynamicClient{
			getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name": "multitier-driver",
						},
						"status": map[string]interface{}{
							"observedGeneration":     int64(1),
							"desiredNumberScheduled": int64(1),
							"numberReady":            int64(1),
							"updatedNumberScheduled": int64(1),
						},
					},
				}, nil
			},
			patchFunc: func(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, fmt.Errorf("internal patch error")
			},
			listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
				return &unstructured.UnstructuredList{
					Items: []unstructured.Unstructured{
						{
							Object: map[string]interface{}{
								"metadata": map[string]interface{}{"name": "multitier-driver-pod-1"},
							},
						},
					},
				}, nil
			},
			deleteFunc: func(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
				deleteCalled = true
				return nil
			},
		}

		restartMTCDriverPods(context.Background(), dynClient, "gke-managed-checkpointing")
		if !deleteCalled {
			t.Errorf("expected delete fallback to be called when patch fails with non-forbidden error")
		}
	})
}
