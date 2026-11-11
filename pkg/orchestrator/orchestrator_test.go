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

package orchestrator

import (
	"slices"
	"strings"
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type stubJobOrchestrator struct {
	JobOrchestrator
	det Detection
}

func TestProviderRegistryAndFactory(t *testing.T) {
	customType := Type("test-custom")
	RegisterProvider(ProviderSpec{
		Type:          customType,
		Prerequisites: []PrereqKind{PrereqKubectl},
		Factory: func(det Detection) JobOrchestrator {
			return &stubJobOrchestrator{det: det}
		},
	})
	t.Cleanup(func() { unregisterProviderForTest(customType) })

	if !slices.Contains(RegisteredTypes(), customType) {
		t.Fatalf("RegisteredTypes() = %v, missing %q", RegisteredTypes(), customType)
	}
	if got := BasicPrerequisites(customType); !slices.Equal(got, []PrereqKind{PrereqKubectl}) {
		t.Fatalf("BasicPrerequisites(%q) = %v, want [kubectl]", customType, got)
	}

	orc, err := NewJobOrchestrator(Detection{Type: customType, KubeContext: "ctx-1"})
	if err != nil {
		t.Fatalf("NewJobOrchestrator() error = %v", err)
	}
	stub, ok := orc.(*stubJobOrchestrator)
	if !ok || stub.det.KubeContext != "ctx-1" {
		t.Fatalf("got %+v, want stubJobOrchestrator with KubeContext=ctx-1", orc)
	}

	if _, err := NewJobOrchestrator(Detection{Type: "unregistered-type"}); err == nil || !strings.Contains(err.Error(), "unregistered-type") {
		t.Fatalf("expected unregistered-type error, got %v", err)
	}
}

func TestRegisterProvider_PanicsOnInvalidSpec(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected RegisterProvider to panic on invalid spec")
		}
	}()
	RegisterProvider(ProviderSpec{})
}

func TestDetector_ProviderDispatchAndFallback(t *testing.T) {
	t.Parallel()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["local-k8s"] = &clientcmdapi.Cluster{Server: "https://10.0.0.1:6443"}
	cfg.Contexts["admin@local-k8s"] = &clientcmdapi.Context{Cluster: "local-k8s"}
	cfg.CurrentContext = "admin@local-k8s"

	mockProvider := ProviderSpec{
		Type: TypeGKE,
		Probe: func(in ProbeInput) (Detection, bool) {
			if in.ClusterName == "matched-gke" {
				return Detection{Type: TypeGKE, KubeContext: "gke-ctx", ClusterName: "matched-gke", Reason: "matched mock probe"}, true
			}
			return Detection{}, false
		},
	}

	det := &Detector{
		Getenv:           func(string) string { return "" },
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return cfg, nil },
		Providers:        []ProviderSpec{mockProvider},
	}

	got, err := det.Detect("matched-gke", "us-central1", "proj")
	if err != nil || got.Type != TypeGKE || got.KubeContext != "gke-ctx" {
		t.Fatalf("Detect(matched-gke) = (%+v, %v), want TypeGKE/gke-ctx", got, err)
	}

	got, err = det.Detect("", "", "")
	if err != nil || got.Type != TypeK8s || got.KubeContext != "admin@local-k8s" || got.ClusterName != "local-k8s" {
		t.Fatalf("Detect(default fallback) = (%+v, %v), want TypeK8s/admin@local-k8s/local-k8s", got, err)
	}

	// Explicit --cluster that does NOT exist in kubeconfig must NOT silently fall back to CurrentContext.
	got, err = det.Detect("nonexistent-cluster", "", "")
	if err != nil || got.Type != TypeK8s || got.KubeContext != "" {
		t.Fatalf("Detect(nonexistent-cluster) = (%+v, %v), want TypeK8s with empty KubeContext", got, err)
	}
}

func TestDetector_EnvOverride(t *testing.T) {
	t.Parallel()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["local-k8s"] = &clientcmdapi.Cluster{Server: "https://10.0.0.1:6443"}
	cfg.Contexts["admin@local-k8s"] = &clientcmdapi.Context{Cluster: "local-k8s"}
	cfg.CurrentContext = "admin@local-k8s"

	probeCalled := 0
	mockProvider := ProviderSpec{
		Type: TypeGKE,
		Probe: func(in ProbeInput) (Detection, bool) {
			probeCalled++
			return Detection{}, false
		},
	}

	detOverride := &Detector{
		Getenv: func(k string) string {
			if k == OrchestratorEnvVar {
				return "k8s"
			}
			return ""
		},
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return cfg, nil },
		Providers:        []ProviderSpec{mockProvider},
	}
	got, err := detOverride.Detect("", "", "")
	if err != nil || got.Type != TypeK8s || got.KubeContext != "admin@local-k8s" || probeCalled != 0 {
		t.Fatalf("Detect(override=k8s) = (%+v, %v), probeCalled=%d", got, err, probeCalled)
	}

	detBad := &Detector{
		Getenv: func(k string) string {
			if k == OrchestratorEnvVar {
				return "mesos"
			}
			return ""
		},
	}
	if _, err := detBad.Detect("", "", ""); err == nil || !strings.Contains(err.Error(), "GCLUSTER_ORCHESTRATOR") {
		t.Fatalf("expected invalid GCLUSTER_ORCHESTRATOR error, got %v", err)
	}
}
