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

var ListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List available target clusters and environments.",
	RunE:         runListClusters,
	SilenceUsage: true,
}

func runListClusters(cmd *cobra.Command, args []string) error {
	logging.Info("Listing clusters...")

	opts := orchestrator.ListOptions{
		ProjectID: projectID,
	}

	clusters, err := orc.ListEnvironments(opts)
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tLOCATION\tSTATUS")
	for _, c := range clusters {
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Name, c.Location, c.Status)
	}
	w.Flush()
	return nil
}
