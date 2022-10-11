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
	"fmt"
	"log"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
)

// Git references when use Makefile
var (
	GitTagVersion string
	GitBranch     string
	GitCommitInfo string
	GitCommitHash string
)

var (
	annotation = make(map[string]string)
	rootCmd    = &cobra.Command{
		Use:   "ghpc",
		Short: "A blueprint and deployment engine for HPC clusters in GCP.",
		Long: `gHPC provides a flexible and simple to use interface to accelerate
HPC deployments on the Google Cloud Platform.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				log.Fatalf("cmd.Help function failed: %s", err)
			}
		},
		Version:     "v1.7.0",
		Annotations: annotation,
	}
)

// Execute the root command
func Execute() error {
	if len(GitCommitInfo) > 0 {
		dir, err := os.Getwd()
		if err == nil {
			repo, err := git.PlainOpen(dir)
			if err == nil {
				head, err := repo.Head()
				if err == nil && GitCommitHash != head.Hash().String() {
					b := head.Name().Short()
					h := head.Hash().String()
					fmt.Fprintf(os.Stderr, "WARNING: ghpc binary was built from a different commit (%s/%s) than the working directory (%s/%s). You can rebuild the binary by running 'make'\n",
						GitBranch, GitCommitHash[0:7], b, h[0:7])
				}
			}
		}

		if len(GitTagVersion) == 0 {
			GitTagVersion = "- not built from official release"
		}
		if len(GitBranch) == 0 {
			GitBranch = "detached HEAD"
		}
		annotation["version"] = GitTagVersion
		annotation["branch"] = GitBranch
		annotation["commitInfo"] = GitCommitInfo
		rootCmd.SetVersionTemplate(`ghpc version {{index .Annotations "version"}}
Built from '{{index .Annotations "branch"}}' branch.
Commit info: {{index .Annotations "commitInfo"}}
`)
	}
	return rootCmd.Execute()
}

func init() {}
