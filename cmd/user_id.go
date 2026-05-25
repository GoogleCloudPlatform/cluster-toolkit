// Copyright 2026 Google LLC
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

package cmd

import (
	"hpc-toolkit/pkg/config"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(userIdCmd)
}

var userIdCmd = &cobra.Command{
	Use:   "get-user-id",
	Short: "Print your User ID (used in Toolkit Telemetry)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		userId := "UNKNOWN"

		if !UserConfigExists {
			if err := config.InitUserConfig(); err == nil {
				UserConfigExists = true
			}
		}

		if UserConfigExists {
			userId = config.GetPersistentUserId()
		}

		cmd.Printf("User ID: %v\n", userId)
		return nil
	},
}
