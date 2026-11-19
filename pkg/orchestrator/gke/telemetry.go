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
	"fmt"
	"strings"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
)

// isGKE reports whether the submission targets a GKE cluster (a project is known) rather than a plain Kubernetes endpoint.
// The CLI always requires a project, so only programmatic callers reach the non-GKE branch today.
// TODO: derive this from the detected target platform once platform detection lands, instead of the project.
func (g *GKEOrchestrator) isGKE(job orchestrator.JobDefinition) bool {
	return strings.TrimSpace(g.projectID) != "" || strings.TrimSpace(job.ProjectID) != ""
}

// ambientTelemetryImage returns GCLUSTER_TELEMETRY_IMAGE unless --ml-diagnostics-image was given explicitly
// (an explicit empty value opts out of the environment default, mirroring --image-pull-secret="").
func ambientTelemetryImage(job orchestrator.JobDefinition) string {
	if job.MLDiagnosticsImageSpecified {
		return ""
	}
	return k8s.TelemetryImageFromEnv()
}

// telemetrySidecarRequested reports whether ML Diagnostics runs as the native collector sidecar: always off GKE,
// and on GKE only when the user names a collector image (otherwise GKE's managed webhook handles it).
func (g *GKEOrchestrator) telemetrySidecarRequested(job orchestrator.JobDefinition) bool {
	if !job.MLDiagnosticsEnabled {
		return false
	}
	return !g.isGKE(job) || strings.TrimSpace(job.MLDiagnosticsImage) != "" || ambientTelemetryImage(job) != ""
}

// telemetrySidecarTrigger names what selected sidecar mode, so a shell-wide GCLUSTER_TELEMETRY_IMAGE that trips the
// TPU-only check on a CPU/GPU job is visible in the error.
func (g *GKEOrchestrator) telemetrySidecarTrigger(job orchestrator.JobDefinition) string {
	switch {
	case strings.TrimSpace(job.MLDiagnosticsImage) != "":
		return "--ml-diagnostics-image"
	case ambientTelemetryImage(job) != "":
		return k8s.TelemetryImageEnvVar
	default:
		return "the non-GKE target"
	}
}

// validateTelemetryFlags is the pure (no cluster access) preflight for the ML Diagnostics options.
func (g *GKEOrchestrator) validateTelemetryFlags(job orchestrator.JobDefinition) error {
	if !job.MLDiagnosticsEnabled {
		if strings.TrimSpace(job.MLDiagnosticsImage) != "" || job.MLDiagnosticsImageSpecified || job.MLDiagnosticsTelemetryPort != 0 {
			return fmt.Errorf("--ml-diagnostics-image and --ml-diagnostics-telemetry-port require --enable-ml-diagnostics")
		}
		return nil
	}
	if job.MLDiagnosticsTelemetryPort == 0 {
		return nil
	}
	if err := k8s.ValidateTelemetryPort(job.MLDiagnosticsTelemetryPort); err != nil {
		return err
	}
	if !g.telemetrySidecarRequested(job) {
		return fmt.Errorf("--ml-diagnostics-telemetry-port is only supported when injecting the ML Diagnostics collector sidecar; pass --ml-diagnostics-image or set %s (or omit --ml-diagnostics-telemetry-port to use the GKE managed webhook)", k8s.TelemetryImageEnvVar)
	}
	return nil
}

// nonTPUTelemetryHint returns the remediation hint when native sidecar mode is triggered on a non-TPU workload.
func (g *GKEOrchestrator) nonTPUTelemetryHint(job orchestrator.JobDefinition) string {
	if !g.isGKE(job) {
		return "omit --enable-ml-diagnostics for non-TPU workloads"
	}
	if strings.TrimSpace(job.MLDiagnosticsImage) != "" {
		return "omit --ml-diagnostics-image to use the GKE managed webhook (or omit --enable-ml-diagnostics)"
	}
	return fmt.Sprintf("pass --ml-diagnostics-image=\"\" or unset %s to use the GKE managed webhook (or omit --enable-ml-diagnostics)", k8s.TelemetryImageEnvVar)
}

// resolveTelemetryCollector returns the native sidecar image and metrics port for job, or an empty image when
// GKE's managed webhook provides ML Diagnostics. Off GKE, an unnamed image is discovered from the TPU node image cache.
func (g *GKEOrchestrator) resolveTelemetryCollector(job orchestrator.JobDefinition) (string, int, error) {
	if err := g.validateTelemetryFlags(job); err != nil {
		return "", 0, err
	}
	if !g.telemetrySidecarRequested(job) {
		return "", 0, nil
	}
	if !config.IsTPU(job.MachineType) && !config.IsTPU(job.ComputeType) {
		return "", 0, fmt.Errorf("ML Diagnostics collector sidecar injection (selected by %s) is only supported for TPU workloads (got compute-type=%q, machine-type=%q); %s", g.telemetrySidecarTrigger(job), job.ComputeType, job.MachineType, g.nonTPUTelemetryHint(job))
	}
	img := strings.TrimSpace(job.MLDiagnosticsImage)
	if img == "" {
		if img = ambientTelemetryImage(job); img != "" {
			logging.Info("Using ML Diagnostics collector image from %s: %s", k8s.TelemetryImageEnvVar, img)
		}
	}
	if img == "" {
		var err error
		if img, err = k8s.DiscoverTelemetryImage(g.usableKubeClient(), job.MachineType); err != nil {
			return "", 0, err
		}
	}
	return img, k8s.EffectiveTelemetryPort(img, job.MLDiagnosticsTelemetryPort), nil
}
