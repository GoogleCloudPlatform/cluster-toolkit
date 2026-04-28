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
	"text/tabwriter"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"

	"github.com/spf13/cobra"
)

var VolumeCmd = &cobra.Command{
	Use:          "volume",
	Short:        "Discovers and lists available storage options accessible to the user.",
	RunE:         runListVolumes,
	SilenceUsage: true,
}

func init() {
	VolumeCmd.Flags().StringVarP(&clusterName, "cluster", "c", "", "Name of the GKE cluster.")
	VolumeCmd.Flags().StringVarP(&location, "location", "l", "", "Location (region or zone) of the GKE cluster.")
}

func runListVolumes(cmd *cobra.Command, args []string) error {
	logging.Info("Listing managed volumes...")

	opts := orchestrator.ListOptions{
		ClusterName:     clusterName,
		ClusterLocation: location,
		ProjectID:       projectID,
	}

	volumes, err := orc.ListVolumes(opts)
	if err != nil {
		return fmt.Errorf("failed to list volumes: %w", err)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tCLUSTER")
	for _, v := range volumes {
		fmt.Fprintf(w, "%s\t%s\t%s\n", v.Name, v.Type, v.Cluster)
	}
	w.Flush()
	return nil
}
