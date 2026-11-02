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

	"github.com/spf13/cobra"
)

// InspectCmd represents the command to inspect GKE clusters.
var InspectCmd = &cobra.Command{
	Use:          "inspect",
	Short:        "Inspect cluster health and workload status.",
	Args:         cobra.NoArgs,
	RunE:         runInspectCmd,
	SilenceUsage: true,
}

var (
	inspectWorkloadName string
	inspectOutputPath   string
	inspectShow         bool
	inspectSPS          bool
)

func init() {
	InspectCmd.Flags().StringVar(&inspectWorkloadName, "name", "", "Specific workload name to inspect.")
	InspectCmd.Flags().StringVarP(&inspectOutputPath, "output", "o", "", "Custom path/filename to write logs to.")
	InspectCmd.Flags().BoolVarP(&inspectShow, "show", "s", false, "Print output to terminal in addition to file.")
	InspectCmd.Flags().BoolVar(&inspectSPS, "sps", false, "Print a short summary of the SPS instance given by --name (status, endpoint, slices, images, sidecar versions) instead of the full report.")
}

func runInspectCmd(cmd *cobra.Command, args []string) error {
	if inspectSPS {
		if inspectWorkloadName == "" {
			return fmt.Errorf("--sps requires --name")
		}
		if inspectOutputPath != "" || inspectShow {
			return fmt.Errorf("--sps prints a summary to the terminal and cannot be used with --output or --show")
		}
	}
	opts := orchestrator.InspectOptions{
		ProjectID:       projectID,
		ClusterName:     clusterName,
		ClusterLocation: location,
		GKENamespace:    gkeNamespace,
		WorkloadName:    inspectWorkloadName,
		OutputPath:      inspectOutputPath,
		Show:            inspectShow,
		SPS:             inspectSPS,
	}

	return orc.InspectCluster(opts)
}
