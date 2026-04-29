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

var DescribeCmd = &cobra.Command{
	Use:          "describe",
	Short:        "Details the specific environment exhaustively (hardware, exact configs, networking).",
	RunE:         runClusterDescribe,
	SilenceUsage: true,
}

func init() {
	DescribeCmd.Flags().StringVarP(&clusterName, "cluster", "c", "", "Name of the GKE cluster. Required.")
	DescribeCmd.Flags().StringVarP(&location, "location", "l", "", "Location (region or zone) of the GKE cluster. Required.")
	_ = DescribeCmd.MarkFlagRequired("cluster")
	_ = DescribeCmd.MarkFlagRequired("location")
}

func runClusterDescribe(cmd *cobra.Command, args []string) error {

	logging.Info("Describing cluster %s...", clusterName)

	opts := orchestrator.ListOptions{
		ProjectID:       projectID,
		ClusterLocation: location,
	}

	description, err := orc.DescribeEnvironment(clusterName, opts)
	if err != nil {
		return fmt.Errorf("failed to describe cluster: %w", err)
	}

	cmd.Println(description)
	return nil
}
