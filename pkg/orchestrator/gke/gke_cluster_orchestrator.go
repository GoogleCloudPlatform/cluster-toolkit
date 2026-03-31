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
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/orchestrator"
	"strings"
)

// ListEnvironments discovers all available target environments.
func (g *GKEOrchestrator) ListEnvironments(opts orchestrator.ListOptions) ([]orchestrator.ClusterStatus, error) {
	result := g.executor.ExecuteCommand("gcloud", "container", "clusters", "list", "--format=json")
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("gcloud container clusters list failed: %s", result.Stderr)
	}

	type Cluster struct {
		Name     string `json:"name"`
		Location string `json:"location"`
		Status   string `json:"status"`
	}

	var clusters []Cluster
	if err := json.Unmarshal([]byte(result.Stdout), &clusters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal clusters list: %w", err)
	}

	var statuses []orchestrator.ClusterStatus
	for _, c := range clusters {
		statuses = append(statuses, orchestrator.ClusterStatus{
			Name:     c.Name,
			Location: c.Location,
			Status:   c.Status,
		})
	}

	return statuses, nil
}

// GetClusterInfo shows summarized status of the current target cluster's resources.
func (g *GKEOrchestrator) GetClusterInfo(name string, opts orchestrator.ListOptions) (string, error) {
	result := g.executor.ExecuteCommand("gcloud", "container", "clusters", "describe", name, "--location="+opts.ClusterLocation, "--format=json")
	if result.ExitCode != 0 {
		return "", fmt.Errorf("gcloud container clusters describe failed: %s", result.Stderr)
	}

	type NodePool struct {
		Name   string `json:"name"`
		Config struct {
			MachineType string `json:"machineType"`
		} `json:"config"`
		Count  int    `json:"count"`
		Status string `json:"status"`
	}
	type Cluster struct {
		NodePools []NodePool `json:"nodePools"`
	}

	var cluster Cluster
	if err := json.Unmarshal([]byte(result.Stdout), &cluster); err != nil {
		return "", fmt.Errorf("failed to unmarshal cluster describe: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Cluster Resource Summary for: %s\n", name))
	sb.WriteString(fmt.Sprintf("Location: %s\n", opts.ClusterLocation))
	sb.WriteString("------------------------------------------\n")
	for _, np := range cluster.NodePools {
		sb.WriteString(fmt.Sprintf("NodePool: %s\n", np.Name))
		sb.WriteString(fmt.Sprintf("  MachineType: %s\n", np.Config.MachineType))
		if np.Count > 0 {
			sb.WriteString(fmt.Sprintf("  Count: %d\n", np.Count))
		}
		sb.WriteString(fmt.Sprintf("  Status: %s\n", np.Status))
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// DescribeEnvironment details the specific environment exhaustively.
func (g *GKEOrchestrator) DescribeEnvironment(name string, opts orchestrator.ListOptions) (string, error) {
	result := g.executor.ExecuteCommand("gcloud", "container", "clusters", "describe", name, "--location="+opts.ClusterLocation, "--format=yaml")
	if result.ExitCode != 0 {
		return "", fmt.Errorf("gcloud container clusters describe failed: %s", result.Stderr)
	}
	return result.Stdout, nil
}

// ListVolumes discovers and lists available storage options (PVCs labeled ghpc_role=file-system).
func (g *GKEOrchestrator) ListVolumes(opts orchestrator.ListOptions) ([]orchestrator.VolumeStatus, error) {
	// Query PVCs with the managed role label
	result := g.executor.ExecuteCommand("kubectl", "get", "pvc", "-l", "ghpc_role=file-system", "-o", "json")
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("kubectl get pvc failed: %s", result.Stderr)
	}

	type PVC struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			StorageClassName *string `json:"storageClassName"`
		} `json:"spec"`
	}
	type PVCList struct {
		Items []PVC `json:"items"`
	}

	var pvcList PVCList
	if err := json.Unmarshal([]byte(result.Stdout), &pvcList); err != nil {
		return nil, fmt.Errorf("failed to unmarshal PVC list: %w", err)
	}

	var volumes []orchestrator.VolumeStatus
	for _, pvc := range pvcList.Items {
		volType := "Standard"
		if module, ok := pvc.Metadata.Labels["ghpc_module"]; ok {
			volType = module
		} else if sc := pvc.Spec.StorageClassName; sc != nil {
			volType = *sc
		}

		volumes = append(volumes, orchestrator.VolumeStatus{
			Name:      pvc.Metadata.Name,
			Type:      volType,
			MountPath: fmt.Sprintf("/mnt/data/%s", pvc.Metadata.Name), // Guessing from PR context
			Cluster:   opts.ClusterName,
		})
	}

	return volumes, nil
}
