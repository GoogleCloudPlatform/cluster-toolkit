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

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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
		executor:         mockExec,
		dynClient:        mockDyn,
		kubeClient:       kube,
		kubeconfigLoader: staticLoader(clientcmdapi.NewConfig()),
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

func TestInitialize_TimeoutAndExecErrors(t *testing.T) {
	primaryCmd := "gcloud container clusters describe my-cluster --location us-central1-a --project my-project --format=json"
	fallbackCmd := "gcloud container clusters describe my-cluster --location us-central1 --project my-project --format=json"

	tests := []struct {
		name          string
		mockResponses map[string][]shell.CommandResult
		wantErrSubstr string
	}{
		{
			name: "Primary describe query times out",
			mockResponses: map[string][]shell.CommandResult{
				primaryCmd: {
					{
						ExitCode: 124,
						Err:      context.DeadlineExceeded,
					},
				},
			},
			wantErrSubstr: fmt.Sprintf("timed out after %v while trying to reach GKE cluster 'my-cluster' in 'us-central1-a'", shell.DefaultCloudAPITimeout),
		},
		{
			name: "Primary describe query fails to execute",
			mockResponses: map[string][]shell.CommandResult{
				primaryCmd: {
					{
						ExitCode: -1,
						Err:      fmt.Errorf("executable file not found in $PATH"),
					},
				},
			},
			wantErrSubstr: "failed to execute gcloud: executable file not found in $PATH",
		},
		{
			name: "Fallback regional describe query times out",
			mockResponses: map[string][]shell.CommandResult{
				primaryCmd: {
					{
						ExitCode: 1,
						Stderr:   "Resource my-cluster was not found in us-central1-a",
					},
				},
				fallbackCmd: {
					{
						ExitCode: 124,
						Err:      context.DeadlineExceeded,
					},
				},
			},
			wantErrSubstr: fmt.Sprintf("timed out after %v while trying to reach GKE cluster 'my-cluster' in fallback region 'us-central1'", shell.DefaultCloudAPITimeout),
		},
		{
			name: "Fallback regional describe query fails to execute",
			mockResponses: map[string][]shell.CommandResult{
				primaryCmd: {
					{
						ExitCode: 1,
						Stderr:   "Resource my-cluster was not found in us-central1-a",
					},
				},
				fallbackCmd: {
					{
						ExitCode: -1,
						Err:      fmt.Errorf("fork/exec gcloud: resource temporarily unavailable"),
					},
				},
			},
			wantErrSubstr: "failed to execute gcloud: fork/exec gcloud: resource temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := newTestGKEOrchestrator(NewMockExecutor(tt.mockResponses))
			_, err := orc.Initialize("my-cluster", "us-central1-a", "my-project")
			if err == nil {
				t.Fatalf("expected Initialize to fail, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("Initialize() error = %q, want substring %q", err.Error(), tt.wantErrSubstr)
			}
		})
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
	var ksaGetCalled bool
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
			name: "Dry-run without HighScaleCheckpointing addon - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				DryRunManifest:         "manifest.yaml",
			},
			wantErr:       true,
			wantErrSubstr: "enable_multi_tier_checkpointing: true",
		},
		{
			name: "CheckpointConfiguration present and legacy KSA annotation - Pass and checks KSA",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
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
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					ksaGetCalled = true
					if name != mtcDriverKSA {
						t.Errorf("unexpected Get(%q), want %q", name, mtcDriverKSA)
					}
					ksa := &unstructured.Unstructured{Object: map[string]interface{}{}}
					ksa.SetAnnotations(map[string]string{"iam.gke.io/gcp-service-account": "node-sa@test-project.iam.gserviceaccount.com"})
					return ksa, nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{
				clusterDesc:      tt.clusterDesc,
				dynClient:        tt.dynClient,
				kubeClient:       tt.kubeClient,
				kubeconfigLoader: staticLoader(clientcmdapi.NewConfig()),
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
	if !ksaGetCalled {
		t.Errorf("expected Get(%q) to be called", mtcDriverKSA)
	}
}

// recordingDynamicClient is a mockDynamicClient that records the resource and
// namespace of the last request.
type recordingDynamicClient struct {
	mockDynamicClient
	resource  schema.GroupVersionResource
	namespace string
}

func (r *recordingDynamicClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	r.resource = gvr
	return &recordingResource{mockNamespaceableResource{getFunc: r.getFunc, listFunc: r.listFunc}, r}
}

type recordingResource struct {
	mockNamespaceableResource
	client *recordingDynamicClient
}

func (r *recordingResource) Namespace(ns string) dynamic.ResourceInterface {
	r.client.namespace = ns
	return &r.mockNamespaceableResource
}

func TestLegacyMTCDriverGSA(t *testing.T) {
	const ksaNamespace, ksaName = "gke-managed-checkpointing", "gke-checkpointing-multitier-node"
	ksaGVR := schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}
	ksaWith := func(annotations map[string]string) *unstructured.Unstructured {
		ksa := &unstructured.Unstructured{Object: map[string]interface{}{}}
		ksa.SetAnnotations(annotations)
		return ksa
	}

	tests := []struct {
		name     string
		noClient bool
		ksa      *unstructured.Unstructured
		getErr   error
		want     string
	}{
		{
			name: "Annotated KSA returns the GSA",
			ksa:  ksaWith(map[string]string{"iam.gke.io/gcp-service-account": "node-sa@test-project.iam.gserviceaccount.com"}),
			want: "node-sa@test-project.iam.gserviceaccount.com",
		},
		{
			name: "KSA without the annotation returns empty",
			ksa:  ksaWith(map[string]string{"unrelated": "value"}),
		},
		{
			name:   "Missing KSA returns empty",
			getErr: apierrors.NewNotFound(schema.GroupResource{Resource: "serviceaccounts"}, ksaName),
		},
		{
			name:   "Forbidden returns empty",
			getErr: apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, ksaName, fmt.Errorf("forbidden")),
		},
		{
			name:     "No usable kubeconfig returns empty",
			noClient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{kubeconfigLoader: staticLoader(clientcmdapi.NewConfig())}
			var gotName string
			client := &recordingDynamicClient{mockDynamicClient: mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					gotName = name
					return tt.ksa, tt.getErr
				},
			}}
			if !tt.noClient {
				orc.dynClient = client
			}

			if got := orc.legacyMTCDriverGSA(); got != tt.want {
				t.Errorf("legacyMTCDriverGSA() = %q, want %q", got, tt.want)
			}
			if tt.noClient {
				return
			}
			if client.resource != ksaGVR || client.namespace != ksaNamespace || gotName != ksaName {
				t.Errorf("Get() targeted %v %s/%s, want %v %s/%s", client.resource, client.namespace, gotName, ksaGVR, ksaNamespace, ksaName)
			}
		})
	}
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

func TestValidateClusterState_DedupesReadinessReads(t *testing.T) {
	versionProbeCalls := 0
	getDepCalls := 0
	rolloutCalls := 0
	priorityClassListCalls := 0

	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "/version") {
				versionProbeCalls++
				return shell.CommandResult{ExitCode: 0}
			}
			if strings.Contains(fullCmd, "kubectl get crd") {
				return shell.CommandResult{ExitCode: 0, Stdout: "found"}
			}
			if strings.Contains(fullCmd, "kubectl get deployment kueue-controller-manager") {
				getDepCalls++
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"metadata":{"generation":1},"spec":{"replicas":1,"template":{"spec":{"containers":[{"name":"manager","image":"registry.k8s.io/kueue/kueue:v0.16.1"}]}}},"status":{"observedGeneration":1,"replicas":1,"updatedReplicas":1,"availableReplicas":1}}`,
				}
			}
			if strings.Contains(fullCmd, "kubectl rollout status") {
				rolloutCalls++
				return shell.CommandResult{ExitCode: 0}
			}
			if strings.Contains(fullCmd, "kubectl get endpointslice") {
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"items":[{"endpoints":[{"addresses":["10.4.1.3"],"conditions":{"ready":true}}]}]}`,
				}
			}
			if strings.HasPrefix(fullCmd, "kubectl get priorityclass") {
				priorityClassListCalls++
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   "system-node-critical high medium",
				}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := newTestGKEOrchestrator(mock)
	orc.namespace = "default"
	orc.dynClient = &mockDynamicClient{
		getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		},
	}
	job := &orchestrator.JobDefinition{
		ClusterName:       "test-cluster",
		ClusterLocation:   "us-central1-a",
		ProjectID:         "test-project",
		PriorityClassName: "high",
	}
	if err := orc.ValidateClusterState(job); err != nil {
		t.Fatalf("ValidateClusterState failed: %v", err)
	}
	if versionProbeCalls != 0 {
		t.Errorf("ValidateClusterState /version probe calls = %d, want 0 (already probed in configureKubectl)", versionProbeCalls)
	}
	if getDepCalls != 1 {
		t.Errorf("ValidateClusterState kueue deployment get calls = %d, want 1", getDepCalls)
	}
	if rolloutCalls != 0 {
		t.Errorf("ValidateClusterState kueue rollout status calls = %d, want 0 when healthy", rolloutCalls)
	}
	if priorityClassListCalls != 1 {
		t.Errorf("ValidateClusterState priorityclass list calls = %d, want 1", priorityClassListCalls)
	}
}
