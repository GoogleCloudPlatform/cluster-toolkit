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
	"text/tabwriter"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"

	"github.com/spf13/cobra"
)

var (
	filterStatus string
	filterName   string
)

var ListWorkloadsCmd = &cobra.Command{
	Use:          "list",
	Short:        "List workloads (jobs) in the cluster.",
	RunE:         runListWorkloads,
	SilenceUsage: true,
}

func init() {
	ListWorkloadsCmd.Flags().StringVar(&filterStatus, "status", "", "Filter jobs by status (e.g. Running, Failed, Succeeded).")
	ListWorkloadsCmd.Flags().StringVar(&filterName, "name-contains", "", "Filter jobs by name.")
}

func runListWorkloads(cmd *cobra.Command, args []string) error {
	logging.Info("Listing jobs...")

	orc, err := gkeOrchestratorFactory()
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	opts := orchestrator.ListOptions{
		ClusterName:     clusterName,
		ClusterLocation: clusterLocation,
		ProjectID:       projectID,
		Status:          filterStatus,
		NameContains:    filterName,
	}

	jobs, err := orc.ListJobs(opts)
	if err != nil {
		return fmt.Errorf("failed to list jobs: %w", err)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME	STATUS	CREATION_TIME	COMPLETION_TIME")
	for _, job := range jobs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", job.Name, job.Status, job.CreationTime, job.CompletionTime)
	}
	w.Flush()
	return nil
}
