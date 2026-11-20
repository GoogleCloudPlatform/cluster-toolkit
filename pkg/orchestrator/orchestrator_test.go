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
	"errors"
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
	inputPrereqs := []PrereqKind{PrereqKubectl}
	RegisterProvider(ProviderSpec{
		Type:          customType,
		Prerequisites: inputPrereqs,
		Factory: func(det Detection) JobOrchestrator {
			return &stubJobOrchestrator{det: det}
		},
	})
	t.Cleanup(func() { unregisterProviderForTest(customType) })

	inputPrereqs[0] = PrereqGKE
	if !slices.Contains(RegisteredTypes(), customType) {
		t.Errorf("RegisteredTypes() = %v, want %q present", RegisteredTypes(), customType)
	}
	gotPrereqs := BasicPrerequisites(customType)
	if !slices.Equal(gotPrereqs, []PrereqKind{PrereqKubectl}) {
		t.Errorf("BasicPrerequisites(%q) = %v, want [kubectl]", customType, gotPrereqs)
	}
	gotPrereqs[0] = PrereqGKE
	if reread := BasicPrerequisites(customType); !slices.Equal(reread, []PrereqKind{PrereqKubectl}) {
		t.Errorf("BasicPrerequisites(%q) after caller mutation = %v, want [kubectl]", customType, reread)
	}

	detIn := Detection{Type: customType, KubeContext: "ctx-1"}
	orc, err := NewJobOrchestrator(detIn)
	if err != nil {
		t.Fatalf("NewJobOrchestrator(%+v) err = %v, want nil", detIn, err)
	}
	stub, ok := orc.(*stubJobOrchestrator)
	if !ok || stub.det.KubeContext != "ctx-1" {
		t.Errorf("NewJobOrchestrator(%+v) = %+v, want *stubJobOrchestrator with KubeContext=\"ctx-1\"", detIn, orc)
	}

	badDet := Detection{Type: "unregistered-type"}
	if _, err := NewJobOrchestrator(badDet); err == nil || !strings.Contains(err.Error(), "unregistered-type") {
		t.Errorf("NewJobOrchestrator(%+v) err = %v, want error containing %q", badDet, err, "unregistered-type")
	}

	// Custom provider registered via RegisterProvider must also work with GCLUSTER_ORCHESTRATOR=<customType>.
	detCustom := &Detector{
		Getenv: func(k string) string {
			if k == EnvVar {
				return string(customType)
			}
			return ""
		},
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return clientcmdapi.NewConfig(), nil },
	}
	gotOverride, err := detCustom.Detect("custom-clus", "", "")
	if err != nil || gotOverride.Type != customType || gotOverride.ClusterName != "custom-clus" {
		t.Errorf("Detect(\"custom-clus\", \"\", \"\") with %s=%q = (%+v, %v), want Type=%q ClusterName=\"custom-clus\"", EnvVar, customType, gotOverride, err, customType)
	}
}

func TestRegisterProvider_PanicsOnInvalidSpec(t *testing.T) {
	t.Parallel()
	validFactory := func(det Detection) JobOrchestrator { return &stubJobOrchestrator{det: det} }
	tests := []struct {
		name string
		spec ProviderSpec
	}{
		{name: "empty_type", spec: ProviderSpec{Factory: validFactory}},
		{name: "nil_factory", spec: ProviderSpec{Type: "missing-factory"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("RegisterProvider(%+v) did not panic, want panic", tc.spec)
				}
			}()
			RegisterProvider(tc.spec)
		})
	}
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
		t.Errorf("Detect(\"matched-gke\", \"us-central1\", \"proj\") = (%+v, %v), want TypeGKE/gke-ctx", got, err)
	}

	got, err = det.Detect("", "", "")
	if err != nil || got.Type != TypeK8s || got.KubeContext != "admin@local-k8s" || got.ClusterName != "local-k8s" {
		t.Errorf("Detect(\"\", \"\", \"\") = (%+v, %v), want TypeK8s/admin@local-k8s/local-k8s", got, err)
	}

	// Explicit --cluster that does NOT exist in kubeconfig must NOT silently fall back to CurrentContext.
	got, err = det.Detect("nonexistent-cluster", "", "")
	if err != nil || got.Type != TypeK8s || got.KubeContext != "" {
		t.Errorf("Detect(\"nonexistent-cluster\", \"\", \"\") = (%+v, %v), want TypeK8s with empty KubeContext", got, err)
	}
	assertDetectorLoadErrorPropagation(t, mockProvider)
}

func assertDetectorLoadErrorPropagation(t *testing.T, mockProvider ProviderSpec) {
	t.Helper()
	detLoadErr := &Detector{
		Getenv: func(string) string { return "" },
		KubeconfigLoader: func() (*clientcmdapi.Config, error) {
			return clientcmdapi.NewConfig(), errors.New("yaml: line 4: mapping values are not allowed")
		},
		Providers: []ProviderSpec{mockProvider},
	}
	if got, err := detLoadErr.Detect("", "", ""); err == nil || !strings.Contains(err.Error(), "failed to load kubeconfig") {
		t.Errorf("Detect(\"\", \"\", \"\") with corrupt kubeconfig = (%+v, %v), want error containing \"failed to load kubeconfig\"", got, err)
	}
	// Provider probe that claims bootstrap (matched-gke) still succeeds even if existing kubeconfig failed to parse.
	if got, err := detLoadErr.Detect("matched-gke", "us-central1", "proj"); err != nil || got.Type != TypeGKE {
		t.Errorf("Detect(\"matched-gke\", ...) with corrupt kubeconfig = (%+v, %v), want TypeGKE", got, err)
	}
}

func envMapGetter(key, val string) func(string) string {
	return func(k string) string {
		if k == key {
			return val
		}
		return ""
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
		Getenv:           envMapGetter(EnvVar, "k8s"),
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return cfg, nil },
		Providers:        []ProviderSpec{mockProvider},
	}
	got, err := detOverride.Detect("", "", "")
	if err != nil || got.Type != TypeK8s || got.KubeContext != "admin@local-k8s" || probeCalled != 0 {
		t.Errorf("Detect(\"\", \"\", \"\") with %s=k8s = (%+v, %v), probeCalled=%d, want TypeK8s/admin@local-k8s/0", EnvVar, got, err, probeCalled)
	}

	danglingCfg := clientcmdapi.NewConfig()
	danglingCfg.CurrentContext = "deleted-ctx"
	detDanglingOverride := &Detector{
		Getenv:           envMapGetter(EnvVar, "k8s"),
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return danglingCfg, nil },
		Providers:        []ProviderSpec{mockProvider},
	}
	gotDangling, err := detDanglingOverride.Detect("", "", "")
	if err != nil || gotDangling.Type != TypeK8s || gotDangling.KubeContext != "" || gotDangling.DanglingContext != "deleted-ctx" {
		t.Errorf("Detect(\"\", \"\", \"\") with %s=k8s and dangling current-context = (%+v, %v), want DanglingContext=\"deleted-ctx\"", EnvVar, gotDangling, err)
	}

	detEmptyOverride := &Detector{
		Getenv:           envMapGetter(EnvVar, "gke"),
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return clientcmdapi.NewConfig(), nil },
		Providers:        []ProviderSpec{mockProvider},
	}
	wantEmpty := Detection{Type: TypeGKE, Forced: true, ClusterName: "fresh-clus", Reason: "forced by GCLUSTER_ORCHESTRATOR=gke"}
	if gotEmpty, err := detEmptyOverride.Detect("fresh-clus", "us-central1", "proj"); err != nil || gotEmpty != wantEmpty {
		t.Errorf("Detect(\"fresh-clus\", ...) with %s=gke on empty kubeconfig = (%+v, %v), want (%+v, nil)", EnvVar, gotEmpty, err, wantEmpty)
	}
	assertDetectorOverrideErrors(t, cfg, mockProvider)
}

func assertDetectorOverrideErrors(t *testing.T, cfg *clientcmdapi.Config, mockProvider ProviderSpec) {
	t.Helper()
	rejectingProvider := ProviderSpec{
		Type: TypeGKE,
		ResolveContext: func(*clientcmdapi.Config, string, string, string) (string, *clientcmdapi.Context, string, bool) {
			return "", nil, "", false
		},
	}
	detRejected := &Detector{
		Getenv:           envMapGetter(EnvVar, "gke"),
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return cfg, nil },
		Providers:        []ProviderSpec{rejectingProvider},
	}
	if gotRejected, err := detRejected.Detect("", "us-central1", "proj"); err != nil || gotRejected.KubeContext != "" {
		t.Errorf("Detect with rejecting provider ResolveContext = (%+v, %v), want KubeContext=\"\" (must not fall back to CurrentContext)", gotRejected, err)
	}

	detCorruptK8s := &Detector{
		Getenv:           envMapGetter(EnvVar, "k8s"),
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return clientcmdapi.NewConfig(), errors.New("permission denied") },
		Providers:        []ProviderSpec{mockProvider},
	}
	if got, err := detCorruptK8s.Detect("", "", ""); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("Detect(\"\", \"\", \"\") with %s=k8s and unreadable kubeconfig = (%+v, %v), want permission denied error", EnvVar, got, err)
	}

	detBad := &Detector{
		Getenv:    envMapGetter(EnvVar, "mesos"),
		Providers: []ProviderSpec{mockProvider},
	}
	if got, err := detBad.Detect("", "", ""); err == nil || !strings.Contains(err.Error(), `"mesos"`) || !strings.Contains(err.Error(), `"gke", "k8s"`) {
		t.Errorf("Detect(\"\", \"\", \"\") with %s=mesos = (%+v, %v), want error naming \"mesos\" and '\"gke\", \"k8s\"'", EnvVar, got, err)
	}
}
