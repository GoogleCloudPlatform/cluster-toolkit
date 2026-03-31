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
	"github.com/spf13/cobra"
)

var (
	clusterName     string
	clusterLocation string
	projectID       string
)

// ClusterCmd represents the base command for cluster-related operations
var ClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "[EXPERIMENTAL] Manage clusters and environments.",
	Long:  `Discover, list, and introspect target clusters and environments. This feature is under active development.`,
}

func init() {
	ClusterCmd.PersistentFlags().StringVar(&clusterName, "cluster", "", "Name of the GKE cluster. Required for info, describe, volume.")
	ClusterCmd.PersistentFlags().StringVar(&clusterLocation, "cluster-location", "", "Location (region or zone) of the GKE cluster. Required for info, describe, volume.")
	ClusterCmd.PersistentFlags().StringVarP(&projectID, "project", "p", "", "Google Cloud Project ID.")

	ClusterCmd.AddCommand(ListCmd)
	ClusterCmd.AddCommand(InfoCmd)
	ClusterCmd.AddCommand(DescribeCmd)
	ClusterCmd.AddCommand(VolumeCmd)
}
