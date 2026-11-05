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

package k8s

import (
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func makeTestTPUNode(name, instanceType, tpuAlloc string, names ...string) unstructured.Unstructured {
	rawNames := make([]any, len(names))
	for i, n := range names {
		rawNames[i] = n
	}
	return unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]any{
				"name":   name,
				"labels": map[string]any{"node.kubernetes.io/instance-type": instanceType},
			},
			"status": map[string]any{
				"allocatable": map[string]any{"google.com/tpu": tpuAlloc},
				"images":      []any{map[string]any{"names": rawNames}},
			},
		},
	}
}

func TestValidateTelemetryPortAndEnvImage(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		if err := ValidateTelemetryPort(port); err == nil || !strings.Contains(err.Error(), "between 1 and 65535") {
			t.Errorf("ValidateTelemetryPort(%d) = %v, want range error", port, err)
		}
	}
	if err := ValidateTelemetryPort(DefaultTelemetryCollectorPort); err != nil {
		t.Errorf("ValidateTelemetryPort(%d) = %v, want nil", DefaultTelemetryCollectorPort, err)
	}
	t.Setenv(TelemetryImageEnvVar, "  reg.local/env-collector:v1  ")
	if got := TelemetryImageFromEnv(); got != "reg.local/env-collector:v1" {
		t.Errorf("TelemetryImageFromEnv() = %q, want trimmed env value", got)
	}
}

func TestDiscoverTelemetryImage_Errors(t *testing.T) {
	const mt = "tpu-v6e-slice"
	noCollector := &stubResourceLister{items: []unstructured.Unstructured{makeTestTPUNode("tpu-0", mt, "4", "reg.local/pause:3.9")}}
	forbidden := &stubResourceLister{err: apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", errors.New("denied"))}
	for _, tc := range []struct {
		name   string
		lister ResourceLister
		want   string
	}{
		{name: "nil client", lister: nil, want: "without an active Kubernetes client"},
		{name: "list error", lister: &stubResourceLister{err: errors.New("boom")}, want: "boom"},
		{name: "forbidden", lister: forbidden, want: "lacking permission to list cluster nodes"},
		{name: "no TPU nodes", lister: &stubResourceLister{}, want: "scaled to zero"},
		{name: "TPU node without collector", lister: noCollector, want: "across 1 TPU node(s)"},
	} {
		_, err := DiscoverTelemetryImage(tc.lister, mt)
		if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "--ml-diagnostics-image") {
			t.Errorf("%s: DiscoverTelemetryImage() error = %v, want substring %q plus the --ml-diagnostics-image hint", tc.name, err, tc.want)
		}
	}
}

func TestDiscoverTelemetryImage_PicksHighestSemver(t *testing.T) {
	const mt = "tpu-v6e-slice"
	// Lexical ordering would pick v1.9.0; semver ordering must pick v1.10.0. Nodes of another instance type
	// and the workload-observability-agent / digest buckets must not outrank the matching node's tagged image.
	nodesLister := &stubResourceLister{
		items: []unstructured.Unstructured{
			makeTestTPUNode("other-mt", "ct5p-hightpu-4t", "4", "reg.local/ai-telemetry-collector:v99.0.0"),
			makeTestTPUNode("matching-mt", mt, "4",
				"reg.local/workload-observability-agent:v9.0.0",
				"reg.local/ai-telemetry-collector@sha256:9999",
				"reg.local/ai-telemetry-collector:v1.9.0",
				"reg.local/ai-telemetry-collector:v1.10.0",
			),
		},
	}
	if got, err := DiscoverTelemetryImage(nodesLister, mt); err != nil || got != "reg.local/ai-telemetry-collector:v1.10.0" {
		t.Errorf("DiscoverTelemetryImage() = (%q, %v), want reg.local/ai-telemetry-collector:v1.10.0", got, err)
	}
	if nodesLister.gotGVR != NodeGVR || nodesLister.gotNS != "" {
		t.Errorf("listed (%v, ns=%q), want cluster-scoped %v", nodesLister.gotGVR, nodesLister.gotNS, NodeGVR)
	}
	if got, err := DiscoverTelemetryImage(nodesLister, "unknown-mt"); err != nil || got != "reg.local/ai-telemetry-collector:v99.0.0" {
		t.Errorf("without a matching instance type, want the newest image across all TPU nodes, got (%q, %v)", got, err)
	}
}

func TestCompareImageRefs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"r/c:v1.10.0", "r/c:v1.9.0", 1},
		{"r/c:1.2.3", "r/c:v1.2.3-rc1", 1},
		{"r/c:20250115", "r/c:20260301", -1},
		{"r/c:latest", "r/c:v0.1.0", -1},
		{"r/c@sha256:aaa", "r/c@sha256:bbb", -1},
	} {
		got := compareImageRefs(tc.a, tc.b)
		if (got > 0) != (tc.want > 0) || (got < 0) != (tc.want < 0) {
			t.Errorf("compareImageRefs(%q, %q) = %d, want sign of %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestTelemetryDefaultsAndPodValidation(t *testing.T) {
	t.Parallel()
	if got := EffectiveTelemetryPort("reg/img:v1", 0); got != DefaultTelemetryCollectorPort {
		t.Errorf("EffectiveTelemetryPort unset = %d, want %d", got, DefaultTelemetryCollectorPort)
	}
	if got := EffectiveTelemetryPort("reg/img:v1", 9095); got != 9095 {
		t.Errorf("EffectiveTelemetryPort explicit = %d, want 9095", got)
	}
	if got := EffectiveTelemetryPort("", 0); got != 0 {
		t.Errorf("EffectiveTelemetryPort without sidecar = %d, want 0", got)
	}
	env := WithTelemetryEnv(map[string]string{"CUSTOM": "1"})
	if env["TPU_RUNTIME_METRICS_PORTS"] != DefaultTPURuntimeMetricsPorts || env["CUSTOM"] != "1" {
		t.Errorf("unexpected WithTelemetryEnv: %+v", env)
	}
	override := WithTelemetryEnv(map[string]string{"TPU_RUNTIME_METRICS_PORTS": "9999"})
	if override["TPU_RUNTIME_METRICS_PORTS"] != "9999" {
		t.Errorf("expected user override to win, got %+v", override)
	}

	inits := []corev1.Container{{Name: TelemetryCollectorContainerName}}
	if err := ValidatePodTelemetry("pod", "hint", "job-1", inits, map[string]string{TPUWorkloadNameLabel: "job-1"}); err != nil {
		t.Errorf("unexpected ValidatePodTelemetry error: %v", err)
	}
	if err := ValidatePodTelemetry("pod", "hint", "job-1", nil, map[string]string{TPUWorkloadNameLabel: "job-1"}); err == nil {
		t.Error("expected missing sidecar error")
	}
	if err := ValidatePodTelemetry("pod", "hint", "job-1", inits, map[string]string{TPUWorkloadNameLabel: "other"}); err == nil || !strings.Contains(err.Error(), TPUWorkloadNameLabel) {
		t.Errorf("expected label mismatch error, got: %v", err)
	}
}
