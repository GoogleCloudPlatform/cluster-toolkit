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
	"slices"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	"github.com/google/go-cmp/cmp"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func makeNodeWithImages(name, tpuAlloc string, imageSets ...[]string) unstructured.Unstructured {
	imgs := make([]any, 0, len(imageSets))
	for _, names := range imageSets {
		nAny := make([]any, len(names))
		for i, s := range names {
			nAny[i] = s
		}
		imgs = append(imgs, map[string]any{"names": nAny})
	}
	alloc := map[string]any{"cpu": "64"}
	if tpuAlloc != "" {
		alloc["google.com/tpu"] = tpuAlloc
	}
	return unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata":   map[string]any{"name": name},
			"status": map[string]any{
				"allocatable": alloc,
				"images":      imgs,
			},
		},
	}
}

func TestMLDiagnostics_WebhookModeAndNonTPUGuard(t *testing.T) {
	setupMockMachineConfig(t)
	cpuResp := shell.CommandResult{ExitCode: 0, Stdout: `{"guestCpus": 4}`}
	gpuResp := shell.CommandResult{ExitCode: 0, Stdout: `{"guestCpus": 48, "memoryMb": 196608, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "nvidia-l4"}]}`}

	t.Run("GKE webhook mode on CPU emits label and zero initContainers", func(t *testing.T) {
		t.Setenv("GCLUSTER_TELEMETRY_IMAGE", "")
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"gcloud compute machine-types describe n2-standard-4 --zone=us-central1-a --format=json": {cpuResp, cpuResp},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.projectID = "gke-proj"
		orc.clusterZones = []string{"us-central1-a"}
		job := orchestrator.JobDefinition{
			WorkloadName:         "cpu-mldiag",
			CommandToRun:         "echo hi",
			ComputeType:          "n2-standard-4",
			MachineType:          "n2-standard-4",
			ProjectID:            "gke-proj",
			ClusterLocation:      "us-central1-a",
			NumSlices:            1,
			NodesPerSlice:        1,
			MLDiagnosticsEnabled: true,
		}
		profile := JobProfile{IsCPUMachine: true, CapacityCount: 4}
		opts, err := orc.PrepareManifestOptions(job, "alpine:3.20", profile, false, false)
		if err != nil {
			t.Fatalf("PrepareManifestOptions failed: %v", err)
		}
		if opts.TelemetryCollectorImage != "" || opts.TerminationGracePeriodSeconds != 0 {
			t.Fatalf("expected empty TelemetryCollectorImage and 0 grace period, got %q / %d", opts.TelemetryCollectorImage, opts.TerminationGracePeriodSeconds)
		}
		_, k8sJob, _ := parseServiceAndJobDocs(t, mustGenerateJobManifest(t, orc, opts, profile))
		if k8sJob.Labels["managed-mldiagnostics-gke"] != "true" {
			t.Errorf("expected managed-mldiagnostics-gke=true, got %+v", k8sJob.Labels)
		}
		if _, hasPodLbl := k8sJob.Spec.Template.Labels["tpu-workload-name"]; hasPodLbl {
			t.Errorf("unexpected tpu-workload-name label in webhook mode: %+v", k8sJob.Spec.Template.Labels)
		}
		if len(k8sJob.Spec.Template.Spec.InitContainers) != 0 {
			t.Errorf("expected zero initContainers in webhook mode, got %+v", k8sJob.Spec.Template.Spec.InitContainers)
		}
	})

	t.Run("rejects native sidecar mode on CPU and GPU workloads", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			projectID   string
			computeType string
			machineType string
			profile     JobProfile
			img         string
			envImg      string
			wantTrigger string
		}{
			{
				name:        "explicit --ml-diagnostics-image on CPU",
				wantTrigger: "--ml-diagnostics-image",
				projectID:   "gke-proj",
				computeType: "n2-standard-4",
				machineType: "n2-standard-4",
				profile:     JobProfile{IsCPUMachine: true, CapacityCount: 4},
				img:         "registry.local/ai-telemetry-collector:v1",
			},
			{
				name:        "GCLUSTER_TELEMETRY_IMAGE on GPU",
				wantTrigger: "GCLUSTER_TELEMETRY_IMAGE",
				projectID:   "gke-proj",
				computeType: "g2-standard-48",
				machineType: "g2-standard-48",
				profile:     JobProfile{CapacityCount: 4},
				envImg:      "registry.local/ai-telemetry-collector:v1",
			},
			{
				name:        "non-GKE --enable-ml-diagnostics on GPU",
				wantTrigger: "non-GKE",
				projectID:   "",
				computeType: "g2-standard-48",
				machineType: "g2-standard-48",
				profile:     JobProfile{CapacityCount: 4},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv("GCLUSTER_TELEMETRY_IMAGE", tc.envImg)
				mockExec := NewMockExecutor(map[string][]shell.CommandResult{
					"gcloud compute machine-types describe n2-standard-4 --zone=us-central1-a --format=json":  {cpuResp},
					"gcloud compute machine-types describe g2-standard-48 --zone=us-central1-a --format=json": {gpuResp},
				})
				orc := newTestGKEOrchestrator(mockExec)
				orc.projectID = tc.projectID
				orc.clusterZones = []string{"us-central1-a"}
				job := orchestrator.JobDefinition{
					WorkloadName:         "non-tpu-sidecar",
					CommandToRun:         "echo hi",
					ComputeType:          tc.computeType,
					MachineType:          tc.machineType,
					ProjectID:            tc.projectID,
					ClusterLocation:      "us-central1-a",
					NumSlices:            1,
					NodesPerSlice:        1,
					MLDiagnosticsEnabled: true,
					MLDiagnosticsImage:   tc.img,
				}
				_, err := orc.PrepareManifestOptions(job, "alpine:3.20", tc.profile, false, false)
				if err == nil || !strings.Contains(err.Error(), "only supported for TPU workloads") || !strings.Contains(err.Error(), tc.wantTrigger) {
					t.Fatalf("expected TPU-only error naming %q, got: %v", tc.wantTrigger, err)
				}
			})
		}
	})
}

func TestMLDiagnostics_NativeSidecarRendering(t *testing.T) {
	setupMockMachineConfig(t)
	tpu7xResp := shell.CommandResult{ExitCode: 0, Stdout: `{"guestCpus": 112, "memoryMb": 196608, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v7x-slice"}]}`}
	for _, tc := range []struct {
		name            string
		port            int
		grace           int
		userEnv         map[string]string
		wantPort        int32
		wantArgs        []string
		wantGrace       int64
		wantMetricsPort string
	}{
		{
			name:            "default port 8889 omits args and passes the 30s grace period through unchanged",
			port:            0,
			grace:           30,
			wantPort:        8889,
			wantArgs:        nil,
			wantGrace:       30,
			wantMetricsPort: "8431,8432,8433,8434,8435,8436,8437,8438",
		},
		{
			name:            "custom port 9095 injects config+endpoint args, keeps a 0s grace period and user TPU_RUNTIME_METRICS_PORTS",
			port:            9095,
			grace:           0,
			userEnv:         map[string]string{"TPU_RUNTIME_METRICS_PORTS": "9431,9432"},
			wantPort:        9095,
			wantArgs:        []string{"--config=/etc/ai-telemetry-collector/config.yaml", "--set=exporters.prometheus.endpoint=0.0.0.0:9095"},
			wantGrace:       0,
			wantMetricsPort: "9431,9432",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockExec := NewMockExecutor(map[string][]shell.CommandResult{
				"gcloud compute machine-types describe tpu7x-standard-4t --zone=us-central1-a --format=json": {tpu7xResp, tpu7xResp},
			})
			orc := newTestGKEOrchestrator(mockExec)
			orc.clusterZones = []string{"us-central1-a"}

			job := orchestrator.JobDefinition{
				WorkloadName:                  "tpu7x-telem",
				CommandToRun:                  "python3 train.py",
				ComputeType:                   "tpu7x-standard-4t",
				MachineType:                   "tpu7x-standard-4t",
				Topology:                      "2x2x1",
				ClusterLocation:               "us-central1-a",
				NumSlices:                     1,
				NodesPerSlice:                 1,
				UseParallelContainers:         true,
				RestartOnExitCodes:            []int{42},
				TerminationGracePeriodSeconds: tc.grace,
				MLDiagnosticsEnabled:          true,
				MLDiagnosticsImage:            "registry.local/ai-telemetry-collector:v1.2.3",
				MLDiagnosticsTelemetryPort:    tc.port,
				Env:                           tc.userEnv,
			}
			profile := JobProfile{CapacityCount: 4}
			opts, err := orc.PrepareManifestOptions(job, "registry.local/jax:v7x", profile, false, false)
			if err != nil {
				t.Fatalf("PrepareManifestOptions failed: %v", err)
			}

			_, k8sJob, _ := parseServiceAndJobDocs(t, mustGenerateJobManifest(t, orc, opts, profile))
			if _, exists := k8sJob.Labels["managed-mldiagnostics-gke"]; exists {
				t.Errorf("managed-mldiagnostics-gke must be omitted when TelemetryCollectorImage is set: %+v", k8sJob.Labels)
			}
			assertPodFailurePolicyContainers(t, k8sJob.Spec.PodFailurePolicy, []string{"workload-container-1", "workload-container-2"})
			assertTelemetryPodTemplate(t, "Job", k8sJob.Spec.Template, "tpu7x-telem", tc.wantPort, tc.wantArgs, tc.wantGrace, tc.wantMetricsPort, 2)

			jsManifest, err := orc.GenerateGKEManifest(opts, profile)
			if err != nil {
				t.Fatalf("GenerateGKEManifest failed: %v", err)
			}
			jsLabels, jsTemplates := parseStrictJobSetTemplates(t, jsManifest)
			if _, exists := jsLabels["managed-mldiagnostics-gke"]; exists {
				t.Errorf("JobSet managed-mldiagnostics-gke must be omitted when sidecar is active: %+v", jsLabels)
			}
			assertPodFailurePolicyContainers(t, jsTemplates["main-job"].Spec.PodFailurePolicy, []string{"workload-container-1", "workload-container-2"})
			assertTelemetryPodTemplate(t, "JobSet", jsTemplates["main-job"].Spec.Template, "tpu7x-telem", tc.wantPort, tc.wantArgs, tc.wantGrace, tc.wantMetricsPort, 2)
		})
	}
}

func assertTelemetryPodTemplate(t *testing.T, kind string, tmpl corev1.PodTemplateSpec, wantName string, wantPort int32, wantArgs []string, wantGrace int64, wantMetricsPorts string, wantContainers int) {
	t.Helper()
	if tmpl.Labels["tpu-workload-name"] != wantName {
		t.Errorf("%s pod label tpu-workload-name = %q, want %q", kind, tmpl.Labels["tpu-workload-name"], wantName)
	}
	if tmpl.Spec.TerminationGracePeriodSeconds == nil || *tmpl.Spec.TerminationGracePeriodSeconds != wantGrace {
		t.Errorf("%s TerminationGracePeriodSeconds = %v, want %d", kind, tmpl.Spec.TerminationGracePeriodSeconds, wantGrace)
	}
	assertTelemetrySidecarSpec(t, kind, tmpl.Spec.InitContainers, wantPort, wantArgs)
	if len(tmpl.Spec.Containers) != wantContainers {
		t.Fatalf("%s Containers len = %d, want %d", kind, len(tmpl.Spec.Containers), wantContainers)
	}
	for _, c := range tmpl.Spec.Containers {
		assertContainerEnvContains(t, c, "TPU_RUNTIME_METRICS_PORTS", wantMetricsPorts)
	}
}

func assertTelemetrySidecarSpec(t *testing.T, kind string, initContainers []corev1.Container, wantPort int32, wantArgs []string) {
	t.Helper()
	if len(initContainers) != 1 {
		t.Fatalf("%s InitContainers len = %d, want 1 (%+v)", kind, len(initContainers), initContainers)
	}
	sc := initContainers[0]
	if sc.Name != "ai-telemetry-collector" || sc.Image != "registry.local/ai-telemetry-collector:v1.2.3" || sc.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("%s sidecar mismatch: %+v", kind, sc)
	}
	if sc.RestartPolicy == nil || *sc.RestartPolicy != corev1.ContainerRestartPolicyAlways {
		t.Errorf("%s sidecar restartPolicy = %v, want Always", kind, sc.RestartPolicy)
	}
	if len(sc.Ports) != 1 || sc.Ports[0].ContainerPort != wantPort || sc.Ports[0].Name != "tpu-metrics" {
		t.Errorf("%s sidecar ports = %+v, want [{tpu-metrics %d}]", kind, sc.Ports, wantPort)
	}
	if diff := cmp.Diff(wantArgs, sc.Args); diff != "" {
		t.Errorf("%s sidecar args mismatch (-want +got):\n%s", kind, diff)
	}
}

func newTelemetryResolutionJob() orchestrator.JobDefinition {
	return orchestrator.JobDefinition{
		WorkloadName:         "telem-res",
		CommandToRun:         "python3 train.py",
		ComputeType:          "v6e-4",
		MachineType:          "tpu-v6e-slice",
		Topology:             "2x2",
		ClusterLocation:      "us-central1-a",
		NumSlices:            1,
		NodesPerSlice:        1,
		MLDiagnosticsEnabled: true,
	}
}

func TestResolveTelemetryCollector_PrecedenceAndNodeCache(t *testing.T) {
	setupMockMachineConfig(t)
	baseTPUJob := newTelemetryResolutionJob()
	profile := JobProfile{CapacityCount: 4}

	t.Run("flag beats GCLUSTER_TELEMETRY_IMAGE and node cache; env ignored when MLDiagnosticsEnabled is false", func(t *testing.T) {
		t.Setenv("GCLUSTER_TELEMETRY_IMAGE", "registry.local/env-collector:v2")
		orc := newCachedTestOrchestrator()
		orc.kubeClient = &MockKubeClient{
			Objects: map[string][]unstructured.Unstructured{
				"nodes": {makeNodeWithImages("tpu-0", "4", []string{"registry.local/ai-telemetry-collector:v0"})},
			},
		}

		disabledJob := baseTPUJob
		disabledJob.MLDiagnosticsEnabled = false
		opts, err := orc.PrepareManifestOptions(disabledJob, "registry.local/jax:v1", profile, false, false)
		if err != nil || opts.TelemetryCollectorImage != "" {
			t.Fatalf("disabled job should ignore GCLUSTER_TELEMETRY_IMAGE: opts=%+v err=%v", opts, err)
		}

		opts, err = orc.PrepareManifestOptions(baseTPUJob, "registry.local/jax:v1", profile, false, false)
		if err != nil || opts.TelemetryCollectorImage != "registry.local/env-collector:v2" {
			t.Fatalf("expected env collector image, got %q (err=%v)", opts.TelemetryCollectorImage, err)
		}

		flagJob := baseTPUJob
		flagJob.MLDiagnosticsImage = "registry.local/flag-collector:v3"
		opts, err = orc.PrepareManifestOptions(flagJob, "registry.local/jax:v1", profile, false, false)
		if err != nil || opts.TelemetryCollectorImage != "registry.local/flag-collector:v3" {
			t.Fatalf("expected flag collector image, got %q (err=%v)", opts.TelemetryCollectorImage, err)
		}

		// --ml-diagnostics-image="" ignores the env var: off GKE the node cache is used instead.
		optOutJob := baseTPUJob
		optOutJob.MLDiagnosticsImageSpecified = true
		opts, err = orc.PrepareManifestOptions(optOutJob, "registry.local/jax:v1", profile, false, false)
		if err != nil || opts.TelemetryCollectorImage != "registry.local/ai-telemetry-collector:v0" {
			t.Fatalf("explicit empty image off GKE should discover from the node cache, got %q (err=%v)", opts.TelemetryCollectorImage, err)
		}
	})

	t.Run("node cache ignores CPU nodes, prefers matching machineType, ranks buckets, and picks the highest semver tag", func(t *testing.T) {
		t.Setenv("GCLUSTER_TELEMETRY_IMAGE", "")
		otherPoolNode := makeNodeWithImages("tpu-other-pool", "4", []string{"registry.local/ai-telemetry-collector:v99.0.0"})
		otherPoolNode.SetLabels(map[string]string{"node.kubernetes.io/instance-type": "ct5p-hightpu-4t"})
		matchingNodeOld := makeNodeWithImages("tpu-0", "4",
			[]string{"registry.local/my-ai-telemetry-collector-helper:v1"},
			[]string{"registry.local/workload-observability-agent:v5"},
			[]string{"registry.local/ai-telemetry-collector@sha256:1111"},
			[]string{"registry.local/ai-telemetry-collector:v1.9.0"},
		)
		matchingNodeOld.SetLabels(map[string]string{"node.kubernetes.io/instance-type": "tpu-v6e-slice"})
		matchingNodeNew := makeNodeWithImages("tpu-1", "4", []string{"registry.local/ai-telemetry-collector:v1.10.0"})
		matchingNodeNew.SetLabels(map[string]string{"node.kubernetes.io/instance-type": "tpu-v6e-slice"})
		_ = unstructured.SetNestedField(matchingNodeNew.Object, int64(4), "status", "allocatable", "google.com/tpu")

		orc := newCachedTestOrchestrator()
		orc.kubeClient = &MockKubeClient{
			Objects: map[string][]unstructured.Unstructured{
				"nodes": {
					makeNodeWithImages("cpu-0", "0", []string{"registry.local/ai-telemetry-collector:v999"}),
					otherPoolNode,
					matchingNodeOld,
					matchingNodeNew,
				},
			},
		}
		opts, err := orc.PrepareManifestOptions(baseTPUJob, "registry.local/jax:v1", profile, false, false)
		if err != nil || opts.TelemetryCollectorImage != "registry.local/ai-telemetry-collector:v1.10.0" {
			t.Fatalf("got collector image %q (err=%v), want registry.local/ai-telemetry-collector:v1.10.0", opts.TelemetryCollectorImage, err)
		}

		orc.kubeClient = &MockKubeClient{
			Objects: map[string][]unstructured.Unstructured{
				"nodes": {makeNodeWithImages("tpu-0", "4",
					[]string{"registry.local/workload-observability-agent@sha256:abcd"},
					[]string{"registry.local/workload-observability-agent:v2"},
				)},
			},
		}
		opts, err = orc.PrepareManifestOptions(baseTPUJob, "registry.local/jax:v1", profile, false, false)
		if err != nil || opts.TelemetryCollectorImage != "registry.local/workload-observability-agent:v2" {
			t.Fatalf("got collector image %q (err=%v), want registry.local/workload-observability-agent:v2", opts.TelemetryCollectorImage, err)
		}
	})
}

// TestValidateTelemetryFlags pins the orchestrator-side guards for programmatic callers that bypass the CLI checks.
func TestValidateTelemetryFlags(t *testing.T) {
	t.Setenv("GCLUSTER_TELEMETRY_IMAGE", "")
	orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{}))
	orc.projectID = "gke-proj"
	for _, tc := range []struct {
		name    string
		job     orchestrator.JobDefinition
		wantErr string
	}{
		{name: "disabled without options", job: orchestrator.JobDefinition{}},
		{name: "disabled with image", job: orchestrator.JobDefinition{MLDiagnosticsImage: "registry.local/col:v1"}, wantErr: "require --enable-ml-diagnostics"},
		{name: "disabled with port", job: orchestrator.JobDefinition{MLDiagnosticsTelemetryPort: 9095}, wantErr: "require --enable-ml-diagnostics"},
		{name: "disabled with explicit empty image", job: orchestrator.JobDefinition{MLDiagnosticsImageSpecified: true}, wantErr: "require --enable-ml-diagnostics"},
		{name: "enabled with default port", job: orchestrator.JobDefinition{MLDiagnosticsEnabled: true}},
		{name: "enabled with port out of range", job: orchestrator.JobDefinition{MLDiagnosticsEnabled: true, MLDiagnosticsImage: "registry.local/col:v1", MLDiagnosticsTelemetryPort: 70000}, wantErr: "between 1 and 65535"},
		{name: "enabled with negative port", job: orchestrator.JobDefinition{MLDiagnosticsEnabled: true, MLDiagnosticsImage: "registry.local/col:v1", MLDiagnosticsTelemetryPort: -1}, wantErr: "between 1 and 65535"},
		{name: "enabled with port but webhook mode on GKE", job: orchestrator.JobDefinition{MLDiagnosticsEnabled: true, ProjectID: "gke-proj", MLDiagnosticsTelemetryPort: 9095}, wantErr: "only supported when injecting"},
		{name: "enabled with port and image", job: orchestrator.JobDefinition{MLDiagnosticsEnabled: true, MLDiagnosticsImage: "registry.local/col:v1", MLDiagnosticsTelemetryPort: 9095}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := orc.validateTelemetryFlags(tc.job)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateTelemetryFlags() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateTelemetryFlags() = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

// TestResolveTelemetryCollector_ExplicitEmptyImageOnGKE pins --ml-diagnostics-image="" ignoring GCLUSTER_TELEMETRY_IMAGE
// on GKE: the managed webhook stays in charge and a custom port is still rejected.
func TestResolveTelemetryCollector_ExplicitEmptyImageOnGKE(t *testing.T) {
	setupMockMachineConfig(t)
	baseTPUJob := newTelemetryResolutionJob()
	profile := JobProfile{CapacityCount: 4}
	t.Setenv("GCLUSTER_TELEMETRY_IMAGE", "registry.local/env-collector:v2")
	orc := newCachedTestOrchestrator()
	orc.projectID = "gke-proj"
	orc.kubeClient = &MockKubeClient{Objects: map[string][]unstructured.Unstructured{
		"nodes": {makeNodeWithImages("tpu-0", "4", []string{"registry.local/ai-telemetry-collector:v0"})},
	}}
	gkeJob := baseTPUJob
	gkeJob.ProjectID = "gke-proj"
	gkeJob.MLDiagnosticsImageSpecified = true
	opts, err := orc.PrepareManifestOptions(gkeJob, "registry.local/jax:v1", profile, false, false)
	if err != nil || opts.TelemetryCollectorImage != "" || !opts.MLDiagnosticsEnabled {
		t.Fatalf("want webhook mode (empty collector image, MLDiagnosticsEnabled), got image=%q enabled=%v err=%v", opts.TelemetryCollectorImage, opts.MLDiagnosticsEnabled, err)
	}
	_, k8sJob, _ := parseServiceAndJobDocs(t, mustGenerateJobManifest(t, orc, opts, profile))
	if k8sJob.Labels["managed-mldiagnostics-gke"] != "true" || len(k8sJob.Spec.Template.Spec.InitContainers) != 0 {
		t.Errorf("want managed-mldiagnostics-gke=true and no initContainers, got labels=%v init=%+v", k8sJob.Labels, k8sJob.Spec.Template.Spec.InitContainers)
	}
	// A custom port still needs sidecar mode; the opt-out must not silently satisfy it via the env var.
	portJob := gkeJob
	portJob.MLDiagnosticsTelemetryPort = 9095
	if _, err := orc.PrepareManifestOptions(portJob, "registry.local/jax:v1", profile, false, false); err == nil || !strings.Contains(err.Error(), "only supported when injecting") {
		t.Fatalf("want port-without-sidecar error, got %v", err)
	}
}

func TestResolveTelemetryCollector_ErrorPaths(t *testing.T) {
	setupMockMachineConfig(t)
	t.Setenv("GCLUSTER_TELEMETRY_IMAGE", "")
	baseTPUJob := newTelemetryResolutionJob()
	profile := JobProfile{CapacityCount: 4}

	orc := newCachedTestOrchestrator()
	if _, err := orc.PrepareManifestOptions(baseTPUJob, "registry.local/jax:v1", profile, false, false); err == nil || !strings.Contains(err.Error(), "no running TPU nodes") {
		t.Errorf("expected zero-TPU-nodes error, got: %v", err)
	}

	orc.kubeClient = &MockKubeClient{
		Objects: map[string][]unstructured.Unstructured{
			"nodes": {makeNodeWithImages("tpu-no-collector", "4", []string{"registry.local/pause:3.9"})},
		},
	}
	if _, err := orc.PrepareManifestOptions(baseTPUJob, "registry.local/jax:v1", profile, false, false); err == nil || !strings.Contains(err.Error(), "across 1 TPU node(s)") {
		t.Errorf("expected TPU-node-without-collector error, got: %v", err)
	}

	orc.kubeClient = &MockKubeClient{
		ListErrs: map[string]error{"nodes": apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", errors.New("forbidden"))},
	}
	if _, err := orc.PrepareManifestOptions(baseTPUJob, "registry.local/jax:v1", profile, false, false); err == nil || !strings.Contains(err.Error(), "lacking permission to list cluster nodes") || !strings.Contains(err.Error(), "--ml-diagnostics-image") {
		t.Errorf("expected actionable --ml-diagnostics-image error on 403 node list, got: %v", err)
	}

	offlineOrc := &GKEOrchestrator{
		kubeClient:       &DefaultKubeClient{},
		machineCapCache:  testMachineCapCache(),
		kubeconfigLoader: func() (*clientcmdapi.Config, error) { return nil, errors.New("must not read host kubeconfig") },
	}
	if _, err := offlineOrc.PrepareManifestOptions(baseTPUJob, "registry.local/jax:v1", profile, false, false); err == nil || !strings.Contains(err.Error(), "--ml-diagnostics-image") {
		t.Errorf("expected actionable offline error without reading host kubeconfig, got: %v", err)
	}

	gkeOrc := newCachedTestOrchestrator()
	gkeOrc.projectID = "gke-proj"
	webhookPortJob := baseTPUJob
	webhookPortJob.ProjectID = "gke-proj"
	webhookPortJob.MLDiagnosticsTelemetryPort = 9090
	if _, err := gkeOrc.PrepareManifestOptions(webhookPortJob, "registry.local/jax:v1", profile, false, false); err == nil || !strings.Contains(err.Error(), "--ml-diagnostics-telemetry-port") {
		t.Errorf("expected webhook custom-port error, got: %v", err)
	}
}

func TestGeneratePathwaysManifest_DualSidecarStrictAcrossModes(t *testing.T) {
	setupMockMachineConfig(t)
	tpuResp := shell.CommandResult{ExitCode: 0, Stdout: `{"guestCpus": 112, "memoryMb": 196608, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v6e-slice"}]}`}

	for _, tc := range []struct {
		name      string
		headless  bool
		deploySPS bool
	}{
		{name: "standard Pathways"},
		{name: "headless Pathways", headless: true},
		{name: "deploy-sps Pathways", deploySPS: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockExec := NewMockExecutor(map[string][]shell.CommandResult{
				"gcloud compute machine-types describe tpu-v6e-slice --zone=us-central1-a --format=json": {tpuResp},
			})
			orc := newTestGKEOrchestrator(mockExec)
			orc.clusterZones = []string{"us-central1-a"}

			pwJob := orchestrator.JobDefinition{
				WorkloadName:                  "pw-dual-sc",
				CommandToRun:                  "python3 train.py",
				ComputeType:                   "v6e-4",
				MachineType:                   "tpu-v6e-slice",
				Topology:                      "2x2",
				NumSlices:                     1,
				NodesPerSlice:                 1,
				ClusterLocation:               "us-central1-a",
				RestartOnExitCodes:            []int{42},
				TerminationGracePeriodSeconds: 30,
				GKEMTCEnabled:                 true,
				GKEMTCRamdiskDirectory:        "/mtc",
				MLDiagnosticsEnabled:          true,
				MLDiagnosticsImage:            "registry.local/ai-telemetry-collector:v1",
				MLDiagnosticsTelemetryPort:    9100,
				IsPathwaysJob:                 true,
				Pathways: orchestrator.PathwaysJobDefinition{
					GCSLocation:                 "gs://pw-bkt",
					HeadNodePool:                "cpu-np",
					Headless:                    tc.headless,
					DeploySPS:                   tc.deploySPS,
					ColocatedPythonSidecarImage: "registry.local/colocated-python:v1",
				},
			}
			fullImg := "registry.local/jax:v1"
			if tc.headless || tc.deploySPS {
				fullImg = ""
				pwJob.CommandToRun = ""
			}

			manifest, err := orc.GeneratePathwaysManifest(pwJob, fullImg, JobProfile{CapacityCount: 4}, false, false)
			if err != nil {
				t.Fatalf("GeneratePathwaysManifest failed: %v", err)
			}
			jsLabels, templates := parseStrictJobSetTemplates(t, manifest)
			if _, hasWebhook := jsLabels["managed-mldiagnostics-gke"]; hasWebhook {
				t.Errorf("unexpected managed-mldiagnostics-gke label: %+v", jsLabels)
			}

			assertPathwaysHeadNoTelemetry(t, templates["pathways-head"], !tc.headless && !tc.deploySPS)
			assertPathwaysWorkerDualSidecars(t, templates["worker"].Spec.Template)
		})
	}
}

func assertPathwaysHeadNoTelemetry(t *testing.T, headJob batchv1.JobTemplateSpec, wantScopedPFP bool) {
	t.Helper()
	if wantScopedPFP {
		assertPodFailurePolicyContainers(t, headJob.Spec.PodFailurePolicy, []string{"workload-container"})
	} else {
		assertPodFailurePolicyContainers(t, headJob.Spec.PodFailurePolicy, nil)
	}
	headTmpl := headJob.Spec.Template
	if _, hasLbl := headTmpl.Labels["tpu-workload-name"]; hasLbl {
		t.Errorf("pathways-head must not carry tpu-workload-name: %+v", headTmpl.Labels)
	}
	for _, c := range slices.Concat(headTmpl.Spec.InitContainers, headTmpl.Spec.Containers) {
		if c.Name == "ai-telemetry-collector" {
			t.Errorf("pathways-head must not run ai-telemetry-collector")
		}
	}
}

func assertPathwaysWorkerDualSidecars(t *testing.T, workerTmpl corev1.PodTemplateSpec) {
	t.Helper()
	if workerTmpl.Labels["tpu-workload-name"] != "pw-dual-sc" {
		t.Errorf("worker tpu-workload-name = %q, want pw-dual-sc", workerTmpl.Labels["tpu-workload-name"])
	}
	if workerTmpl.Spec.TerminationGracePeriodSeconds == nil || *workerTmpl.Spec.TerminationGracePeriodSeconds != 30 {
		t.Errorf("worker TerminationGracePeriodSeconds = %v, want 30", workerTmpl.Spec.TerminationGracePeriodSeconds)
	}
	var initNames []string
	for _, ic := range workerTmpl.Spec.InitContainers {
		initNames = append(initNames, ic.Name)
	}
	if diff := cmp.Diff([]string{"colocated-python-sidecar", "ai-telemetry-collector"}, initNames); diff != "" {
		t.Fatalf("worker initContainers mismatch (-want +got):\n%s", diff)
	}
	telemSC := workerTmpl.Spec.InitContainers[1]
	wantArgs := []string{"--config=/etc/ai-telemetry-collector/config.yaml", "--set=exporters.prometheus.endpoint=0.0.0.0:9100"}
	if len(telemSC.Ports) != 1 || telemSC.Ports[0].ContainerPort != 9100 || cmp.Diff(wantArgs, telemSC.Args) != "" {
		t.Errorf("worker telemetry sidecar port/args mismatch: %+v", telemSC)
	}
	assertContainerEnvContains(t, workerTmpl.Spec.Containers[0], "TPU_RUNTIME_METRICS_PORTS", "8431,8432,8433,8434,8435,8436,8437,8438")
}

func assertContainerEnvContains(t *testing.T, c corev1.Container, key, wantVal string) {
	t.Helper()
	for _, e := range c.Env {
		if e.Name == key {
			if e.Value != wantVal {
				t.Errorf("container %s env %s = %q, want %q", c.Name, key, e.Value, wantVal)
			}
			return
		}
	}
	t.Errorf("container %s missing env %s (got %+v)", c.Name, key, c.Env)
}
