// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gke

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestDiscoverK8sInventory_NodeConstraintInvariant(t *testing.T) {
	t.Setenv(k8s.LabelSchemeEnvVar, "")

	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", errors.New("forbidden"))
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get --raw /apis/slice.tpu.google.com --request-timeout=10s": {{ExitCode: 1, Stderr: "Error from server (NotFound): 404 page not found"}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.kubeContext = "generic-ctx"
	orc.kubeClient = &MockKubeClient{
		ListErrs: map[string]error{"nodes": forbidden},
	}

	job := orchestrator.JobDefinition{
		ComputeType:    "tpu7x",
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "some-slice"},
	}
	sm := &StorageManager{orchestrator: orc}
	if err := orc.validateSubmitLocalPreflight(sm, job); err != nil {
		t.Fatalf("validateSubmitLocalPreflight failed: %v", err)
	}
	inv, err := orc.DiscoverK8sInventory("")
	if err != nil {
		t.Fatalf("DiscoverK8sInventory failed: %v", err)
	}
	if !inv.ExplicitMode || inv.LabelScheme != k8s.SchemeGeneric {
		t.Fatalf("got ExplicitMode=%v LabelScheme=%q, want (true, generic)", inv.ExplicitMode, inv.LabelScheme)
	}
	if orc.inventory != inv {
		t.Errorf("orc.inventory did not cache discovered inventory")
	}
	mockExec.assertAllKubectlPinned(t, "generic-ctx")
	if len(mockExec.timeouts) != 1 || mockExec.timeouts[0] != apiGroupProbeTimeout {
		t.Errorf("timeouts = %v, want [%v]", mockExec.timeouts, apiGroupProbeTimeout)
	}
}

func TestDiscoverK8sInventory_TPUSliceExplicitModeAndCacheInvalidation(t *testing.T) {
	t.Setenv(k8s.LabelSchemeEnvVar, "")

	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", errors.New("forbidden"))
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get --raw /apis/slice.tpu.google.com --request-timeout=10s": {{ExitCode: 0, Stdout: "{}"}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.kubeContext = "tpu-slice-ctx"
	orc.kubeClient = &MockKubeClient{
		ListErrs: map[string]error{
			"nodes":          forbidden,
			"slices":         forbidden,
			"partitiontrees": forbidden,
		},
	}

	inv, err := orc.DiscoverK8sInventory("")
	if err != nil {
		t.Fatalf("DiscoverK8sInventory failed: %v", err)
	}
	if !inv.ExplicitMode || inv.LabelScheme != k8s.SchemeTPUSlice || orc.inventory != inv {
		t.Fatalf("got ExplicitMode=%v LabelScheme=%q cached=%v, want (true, tpu-slice, true)", inv.ExplicitMode, inv.LabelScheme, orc.inventory == inv)
	}
	mockExec.assertAllKubectlPinned(t, "tpu-slice-ctx")

	orc.mode = orchestrator.TypeK8s
	if err := orc.SubmitJob(orchestrator.JobDefinition{}); err == nil {
		t.Fatalf("expected UnsupportedK8sSubmitError for TypeK8s")
	}
	if orc.inventory != nil {
		t.Errorf("TypeK8s SubmitJob early return must clear cached inventory")
	}
}

func TestDiscoverK8sInventory_LabelSchemeAndHardwareResolverWiring(t *testing.T) {
	tpuNode := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name": "gke-tpu-0",
				"labels": map[string]interface{}{
					k8s.GKENodePoolLabel:       "tpu-pool",
					k8s.GKETPUAcceleratorLabel: "v5lite-4",
					k8s.GKETPUTopologyLabel:    "2x2",
				},
			},
			"status": map[string]interface{}{
				"allocatable": map[string]interface{}{
					"cpu":            "112",
					"memory":         "176Gi",
					"google.com/tpu": "4",
				},
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "True"},
				},
			},
		},
	}

	t.Setenv(k8s.LabelSchemeEnvVar, "gke")
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.kubeContext = "gke-ctx"
	orc.kubeClient = &MockKubeClient{
		Objects: map[string][]unstructured.Unstructured{"nodes": {tpuNode}},
	}

	invEnv, err := orc.DiscoverK8sInventory("")
	if err != nil {
		t.Fatalf("DiscoverK8sInventory(env=gke) failed: %v", err)
	}
	if invEnv.LabelScheme != k8s.SchemeGKE || invEnv.TPU == nil || invEnv.TPU.TPUType != "v5e" || invEnv.TPU.MachineType != "ct5lp-hightpu-4t" {
		t.Fatalf("unexpected env-wired TPU inventory: scheme=%q TPU=%+v", invEnv.LabelScheme, invEnv.TPU)
	}

	invFlag, err := orc.DiscoverK8sInventory("generic")
	if err != nil {
		t.Fatalf("DiscoverK8sInventory(flag=generic) failed: %v", err)
	}
	if invFlag.LabelScheme != k8s.SchemeGeneric || invFlag.TPU == nil || invFlag.TPU.MachineType != "ct5lp-hightpu-4t" {
		t.Fatalf("unexpected flag-overridden TPU inventory: scheme=%q TPU=%+v", invFlag.LabelScheme, invFlag.TPU)
	}
}

func TestDiscoverK8sInventory_ClientLifecycleAndNoAmbientProbe(t *testing.T) {
	t.Setenv(k8s.LabelSchemeEnvVar, "")

	mockExec := NewMockExecutor(nil)
	orc := newTestGKEOrchestrator(mockExec)
	orc.kubeClient = &MockKubeClient{
		ListErrs: map[string]error{
			"slices": apierrors.NewNotFound(k8s.TPUSliceGVR.GroupResource(), ""),
		},
	}
	inv, err := orc.DiscoverK8sInventory("")
	if err != nil || inv.LabelScheme != k8s.SchemeGeneric {
		t.Fatalf("blank-context discovery = (%+v, %v), want generic without ambient probe", inv, err)
	}
	if len(mockExec.calls) != 0 {
		t.Errorf("expected zero kubectl calls when kubeContext is blank, got %v", mockExec.calls)
	}

	var typedNil *DefaultKubeClient
	orc.SetKubeClient(typedNil)
	if orc.kubeClient != nil || orc.dynClient != nil || orc.inventory != nil {
		t.Errorf("SetKubeClient(typedNil) did not clear kubeClient/dynClient/inventory")
	}
	if _, err := orc.DiscoverK8sInventory(""); !errors.Is(err, errKubeClientUninitialized) {
		t.Errorf("expected errKubeClientUninitialized after typed-nil reset, got %v", err)
	}
}

func TestDiscoverK8sInventory_ErrorPropagation(t *testing.T) {
	t.Setenv(k8s.LabelSchemeEnvVar, "")
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.inventory = &k8s.Inventory{LabelScheme: k8s.SchemeGeneric}
	if _, err := orc.DiscoverK8sInventory("invalid"); err == nil || !strings.Contains(err.Error(), "invalid label scheme") {
		t.Errorf("expected source-neutral invalid label scheme error, got: %v", err)
	} else if strings.Contains(err.Error(), k8s.LabelSchemeEnvVar) || strings.Contains(err.Error(), "--k8s-label-scheme") {
		t.Errorf("flag-value error must not misattribute to env/flag: %v", err)
	}
	if orc.inventory != nil {
		t.Errorf("expected failed DiscoverK8sInventory to clear stale cached inventory")
	}

	t.Setenv(k8s.LabelSchemeEnvVar, "bogus")
	if _, err := orc.DiscoverK8sInventory(""); err == nil || !strings.Contains(err.Error(), k8s.LabelSchemeEnvVar) || strings.Contains(err.Error(), "--k8s-label-scheme") {
		t.Errorf("expected GCLUSTER_LABEL_SCHEME error without --k8s-label-scheme, got: %v", err)
	}
	t.Setenv(k8s.LabelSchemeEnvVar, "")

	orcNoClient := NewGKEOrchestrator()
	if _, err := orcNoClient.DiscoverK8sInventory(""); !errors.Is(err, errKubeClientUninitialized) {
		t.Errorf("expected errKubeClientUninitialized, got %v", err)
	}
}

func TestDiscoverK8sInventory_LazyManagedClientInit(t *testing.T) {
	t.Parallel()
	orcBrokenKube := NewGKEOrchestrator()
	orcBrokenKube.kubeContext = "broken-ctx"
	orcBrokenKube.kubeconfigLoader = func() (*clientcmdapi.Config, error) {
		return nil, errors.New("unreadable kubeconfig")
	}
	if _, err := orcBrokenKube.DiscoverK8sInventory(""); err == nil || !strings.Contains(err.Error(), "unreadable kubeconfig") {
		t.Errorf("expected wrapped getDynamicClient error, got: %v", err)
	}

	orcLazy := NewGKEOrchestrator()
	orcLazy.kubeContext = testContext
	orcLazy.kubeconfigLoader = staticLoader(kubeconfigWith("https://127.0.0.1:6443", nil, nil, testContext))
	kc, err := orcLazy.requireKubeClientForDiscovery()
	if err != nil || kc == nil || orcLazy.dynClient == nil || !orcLazy.dynClientManaged || orcLazy.dynClientContext != testContext {
		t.Errorf("lazy managed client init = (kc=%v, err=%v, managed=%v, ctx=%q), want initialized managed client", kc != nil, err, orcLazy.dynClientManaged, orcLazy.dynClientContext)
	}
}

func primeOrchInventoryAndDynClient(orc *GKEOrchestrator, id string, managed bool) dynamic.Interface {
	dyn := &stubDynClient{id: id}
	if managed {
		orc.dynClient = dyn
		orc.dynClientManaged = true
		orc.dynClientContext = "old-ctx"
	} else {
		orc.SetDynamicClient(dyn)
	}
	orc.inventory = &k8s.Inventory{LabelScheme: k8s.SchemeTPUSlice}
	return dyn
}

func assertConfigureClearedState(t *testing.T, label string, orc *GKEOrchestrator, injected dynamic.Interface, wantKeep bool) {
	t.Helper()
	if orc.inventory != nil {
		t.Errorf("%s: must clear cached inventory", label)
	}
	if wantKeep && orc.dynClient != injected {
		t.Errorf("%s: must preserve externally injected dynClient", label)
	}
	if !wantKeep && (orc.dynClient != nil || orc.dynClientManaged || orc.dynClientContext != "") {
		t.Errorf("%s: must invalidate managed dynClient", label)
	}
}

func TestConfigureKubectlAndK8sKubectl_InventoryAndDynClientLifecycle(t *testing.T) {
	t.Parallel()
	const probeCmd = "kubectl get --raw /version --request-timeout=5s"
	for _, managed := range []bool{true, false} {
		exec := NewMockExecutor(map[string][]shell.CommandResult{
			probeCmd: {{ExitCode: 0, Stdout: `{"gitVersion":"v1.31.0"}`}},
		})
		orc := newTestGKEOrchestrator(exec)
		orc.clusterDesc = testClusterDesc(true, true)
		orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))
		dyn := primeOrchInventoryAndDynClient(orc, "gke", managed)
		if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
			t.Fatalf("configureKubectl(managed=%v) error = %v", managed, err)
		}
		assertConfigureClearedState(t, fmt.Sprintf("configureKubectl(managed=%v)", managed), orc, dyn, !managed)

		k8sOrc := NewGKEOrchestrator()
		k8sOrc.kubeContext = "existing-ctx"
		k8sOrc.k8sCluster = "existing-cluster"
		k8sDyn := primeOrchInventoryAndDynClient(k8sOrc, "k8s", managed)
		if err := k8sOrc.configureK8sKubectl("existing-cluster"); err != nil {
			t.Fatalf("configureK8sKubectl(managed=%v) error = %v", managed, err)
		}
		assertConfigureClearedState(t, fmt.Sprintf("configureK8sKubectl(managed=%v)", managed), k8sOrc, k8sDyn, !managed)
	}
}
