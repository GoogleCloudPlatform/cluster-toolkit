/*
Copyright 2022 Google LLC

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

// Package cmd defines command line utilities for ghpc
package cmd

import (
	"log"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "ghpc",
		Short: "A blueprint and deployment engine for HPC clusters in GCP.",
		Long: `gHPC provides a flexible and simple to use interface to accelerate
HPC deployments on the Google Cloud Platform.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				log.Fatalf("cmd.Help function failed: %s", err)
			}
		},
		Version: "v1.4.1",
	}
)

// Execute the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {}
