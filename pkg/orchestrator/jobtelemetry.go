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
	"encoding/json"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MetricEntry holds telemetry data for internal Google usage.
type MetricEntry struct {
	Timestamp             time.Time         `json:"timestamp"`
	WorkloadName          string            `json:"workload_name"`
	LatencySeconds        float64           `json:"latency_seconds"`
	SubmissionSuccess     bool              `json:"submission_success"`
	StaticResourceProfile map[string]string `json:"static_resource_profile"`
}

var (
	isInternalCached bool
	isInternalOnce   sync.Once
)

// isInternalUser checks if the active gcloud account is a @google.com domain.
func isInternalUser() bool {
	isInternalOnce.Do(func() {
		res := shell.ExecuteCommand("gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
		if res.ExitCode != 0 {
			isInternalCached = false
			return
		}
		account := strings.TrimSpace(res.Stdout)
		isInternalCached = strings.HasSuffix(account, "@google.com")
	})
	return isInternalCached
}

// RecordLocalMetrics appends a telemetry metric entry to ~/.gcluster/job_telemetry_metrics.jsonl only for internal Googlers.
func RecordLocalMetrics(workloadName string, latency float64, success bool, profile map[string]string) {
	if os.Getenv("GCLUSTER_SKIP_TELEMETRY") == "true" {
		logging.Info("Skipping telemetry metrics due to GCLUSTER_SKIP_TELEMETRY environment variable.")
		return
	}

	if !isInternalUser() {
		return // Skip quietly if not an internal Googler
	}

	entry := MetricEntry{
		Timestamp:             time.Now(),
		WorkloadName:          workloadName,
		LatencySeconds:        latency,
		SubmissionSuccess:     success,
		StaticResourceProfile: profile,
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		logging.Error("Failed to resolve user home for internal telemetry: %v", err)
		return
	}

	metricsDir := filepath.Join(homeDir, ".gcluster")
	if err := os.MkdirAll(metricsDir, 0755); err != nil {
		logging.Error("Failed to create metrics storage directory %s: %v", metricsDir, err)
		return
	}

	metricsFile := filepath.Join(metricsDir, "job_telemetry_metrics.jsonl")

	data, err := json.Marshal(entry)
	if err != nil {
		logging.Error("Failed to marshal telemetry metrics to JSON: %v", err)
		return
	}

	f, err := os.OpenFile(metricsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logging.Error("Failed to open telemetry metrics file %s: %v", metricsFile, err)
		return
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		logging.Error("Failed to write telemetry metrics to %s: %v", metricsFile, err)
	}
}
