// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package job

import (
	"fmt"
	"os"
	"path/filepath"

	"hpc-toolkit/pkg/orchestrator/gke"
	"hpc-toolkit/pkg/shell"

	"github.com/spf13/cobra"
)

var (
	outputDir    string
	overwriteDst bool
)

var gkeTemplateExtractCmd = &cobra.Command{
	Use:   "gke-template-extract",
	Short: "Extract all default GKE templates to a local directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		parentDir := filepath.Dir(outputDir)
		if _, err := os.Stat(parentDir); err != nil {
			if os.IsNotExist(err) {
				prompt := fmt.Sprintf("Directory %q does not exist. Would you like to create it?", parentDir)
				if !shell.PromptYesNo(prompt) {
					return fmt.Errorf("directory %q does not exist. Please check your path for typos or create the directory manually", parentDir)
				}
			} else {
				return fmt.Errorf("failed to check directory %s: %w", parentDir, err)
			}
		}
		return gke.ExtractDefaultTemplates(outputDir, overwriteDst)
	},
}

func init() {
	JobCmd.AddCommand(gkeTemplateExtractCmd) // Register under gcluster job

	gkeTemplateExtractCmd.Flags().StringVar(&outputDir, "output-dir", "./gke-templates", "The target directory to extract the templates to.")
	gkeTemplateExtractCmd.Flags().BoolVar(&overwriteDst, "overwrite", false, "Overwrite existing templates in the target directory.")
}
