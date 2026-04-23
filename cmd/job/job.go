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
	"fmt"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/gke"

	"github.com/spf13/cobra"
)

var (
	clusterName string
	location    string
	projectID   string
)

var gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
	return gke.NewGKEOrchestrator()
}

var orc orchestrator.JobOrchestrator

// JobCmd represents the base command for job-related operations
var JobCmd = &cobra.Command{
	Use:   "job",
	Short: "[EXPERIMENTAL/ALPHA] Manage jobs on the cluster. Alpha version and not yet supported for production use.",
	Long:  `[EXPERIMENTAL/ALPHA] Manage jobs on the cluster. This is the alpha version of the feature and is under active development. The feature is not yet supported for production use.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		orc = gkeOrchestratorFactory()

		ctx := loadContext()
		if clusterName == "" {
			clusterName = ctx.ClusterName
		}
		if location == "" {
			location = ctx.Location
		}
		if projectID == "" {
			projectID = ctx.ProjectID
		}

		if clusterName == "" {
			return fmt.Errorf("cluster name is required; please specify it using the --cluster flag or set a default value using 'gcluster job config set cluster <value>'")
		}
		if location == "" {
			return fmt.Errorf("location is required; please specify it using the --location flag or set a default value using 'gcluster job config set location <value>'")
		}
		if projectID == "" {
			return fmt.Errorf("project ID is required; please specify it using the --project flag or set a default value using 'gcluster job config set project <value>'")
		}

		return nil
	},
}

func init() {
	JobCmd.PersistentFlags().StringVarP(&clusterName, "cluster", "c", "", "Name of the GKE cluster.")
	JobCmd.PersistentFlags().StringVarP(&location, "location", "l", "", "Location (region or zone) of the GKE cluster.")
	JobCmd.PersistentFlags().StringVarP(&projectID, "project", "p", "", "Google Cloud Project ID.")

	JobCmd.AddCommand(SubmitCmd)
	JobCmd.AddCommand(CancelJobCmd)
	JobCmd.AddCommand(ListWorkloadsCmd)
	JobCmd.AddCommand(LogsCmd)
	JobCmd.AddCommand(ConfigCmd)
}
