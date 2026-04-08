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

package job

import (
	"hpc-toolkit/pkg/orchestrator/gke"

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

// JobCmd represents the base command for job-related operations
var JobCmd = &cobra.Command{
	Use:   "job",
	Short: "[EXPERIMENTAL/ALPHA] Manage jobs on the cluster. Alpha version and not yet supported for production use.",
	Long:  `[EXPERIMENTAL/ALPHA] Manage jobs on the cluster. This is the alpha version of the feature and is under active development. The feature is not yet supported for production use.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		orc = gkeOrchestratorFactory()
		return nil
	},
}

func init() {
	JobCmd.PersistentFlags().StringVarP(&clusterName, "cluster", "c", "", "Name of the GKE cluster. Required.")
	JobCmd.PersistentFlags().StringVarP(&location, "location", "l", "", "Location (region or zone) of the GKE cluster. Required.")
	JobCmd.PersistentFlags().StringVarP(&projectID, "project", "p", "", "Google Cloud Project ID.")

	_ = JobCmd.MarkPersistentFlagRequired("cluster")
	_ = JobCmd.MarkPersistentFlagRequired("location")

	JobCmd.AddCommand(SubmitCmd)
	JobCmd.AddCommand(CancelJobCmd)
	JobCmd.AddCommand(ListWorkloadsCmd)
	JobCmd.AddCommand(LogsCmd)
}
