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
	"hpc-toolkit/pkg/orchestrator"

	"github.com/spf13/cobra"
)

var LogsCmd = &cobra.Command{
	Use:          "logs [job-name]",
	Short:        "Fetch logs for a job in the cluster.",
	Args:         cobra.ExactArgs(1),
	RunE:         runLogsCmd,
	SilenceUsage: true,
}

var follow bool

func init() {
	LogsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream logs continuously")
}

func runLogsCmd(cmd *cobra.Command, args []string) error {
	jobName := args[0]

	opts := orchestrator.LogsOptions{
		ClusterName:     clusterName,
		ClusterLocation: location,
		ProjectID:       projectID,
		Follow:          follow,
	}

	output, err := orc.GetJobLogs(jobName, opts)
	if err != nil {
		return err
	}

	cmd.Println(output)

	return nil
}
