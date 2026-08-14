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

package cluster

import (
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator/gke"
	"hpc-toolkit/pkg/shell"
	"strings"

	"github.com/spf13/cobra"
)

var (
	clusterName string
	location    string
	projectID   string
)

var gkeOrchestratorFactory = func() *gke.GKEOrchestrator {
	return gke.NewGKEOrchestrator()
}

var orc *gke.GKEOrchestrator

// ClusterCmd represents the base command for cluster-related operations
var ClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "[EXPERIMENTAL] Manage clusters and environments.",
	Long:  `Discover, list, and introspect target clusters and environments. This feature is under active development.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		orc = gkeOrchestratorFactory()

		if projectID == "" {
			result := shell.ExecuteCommand("gcloud", "config", "get-value", "project")
			ambientProject := strings.TrimSpace(result.Stdout)

			if result.ExitCode != 0 || ambientProject == "" {
				return fmt.Errorf("no Google Cloud project specified. Please provide one via the '--project' flag or set a default project using 'gcloud config set project <PROJECT_ID>'")
			}

			projectID = ambientProject
			logging.Info("Using ambient project ID: %s", projectID)
		}
		return nil
	},
}

func init() {
	ClusterCmd.PersistentFlags().StringVarP(&projectID, "project", "p", "", "Google Cloud Project ID.")

	ClusterCmd.AddCommand(ListCmd)
	ClusterCmd.AddCommand(InfoCmd)
	ClusterCmd.AddCommand(DescribeCmd)
	ClusterCmd.AddCommand(VolumeCmd)
}
