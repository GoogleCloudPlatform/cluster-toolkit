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
	"hpc-toolkit/pkg/orchestrator"

	"github.com/spf13/cobra"
)

var InfoCmd = &cobra.Command{
	Use:          "info",
	Short:        "Show summarized status of the current target cluster's resources.",
	RunE:         runClusterInfo,
	SilenceUsage: true,
}

func init() {
	InfoCmd.Flags().StringVarP(&clusterName, "cluster", "c", "", "Name of the GKE cluster. Required.")
	InfoCmd.Flags().StringVarP(&location, "location", "l", "", "Location (region or zone) of the GKE cluster. Required.")
	_ = InfoCmd.MarkFlagRequired("cluster")
	_ = InfoCmd.MarkFlagRequired("location")
}

func runClusterInfo(cmd *cobra.Command, args []string) error {

	logging.Info("Fetching cluster info for %s...", clusterName)

	opts := orchestrator.ListOptions{
		ProjectID:       projectID,
		ClusterLocation: location,
	}

	info, err := orc.GetClusterInfo(clusterName, opts)
	if err != nil {
		return fmt.Errorf("failed to get cluster info: %w", err)
	}

	cmd.Println(info)
	return nil
}
