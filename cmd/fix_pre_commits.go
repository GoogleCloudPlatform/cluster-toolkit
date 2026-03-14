/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"hpc-toolkit/pkg/ai"
	"hpc-toolkit/pkg/logging"

	"github.com/spf13/cobra"
)

var (
	maxRetries int
	verbose    bool
	region     string
	model      string
)

var fixPreCommitsCmd = &cobra.Command{
	Use:   "fix-pre-commits",
	Short: "Automatically fix pre-commit failures using AI",
	Long:  `Runs pre-commit hooks, identifies failures, and uses AI to generate and apply fixes.`,
	Run: func(cmd *cobra.Command, args []string) {
		fixer := ai.NewFixer(ai.FixerOptions{
			MaxRetries: maxRetries,
			Verbose:    verbose,
			Region:     region,
			Model:      model,
		})
		if err := fixer.Run(args); err != nil {
			logging.Fatal("Failed to fix pre-commits: %v", err)
		}
	},
}

func init() {
	fixPreCommitsCmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Maximum number of retries per file")
	fixPreCommitsCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	fixPreCommitsCmd.Flags().StringVar(&region, "region", "us-central1", "Vertex AI region")
	fixPreCommitsCmd.Flags().StringVar(&model, "model", "gemini-2.0-flash-001", "Vertex AI model")
	aiCmd.AddCommand(fixPreCommitsCmd)
}
