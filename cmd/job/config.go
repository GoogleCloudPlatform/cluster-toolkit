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
	"hpc-toolkit/pkg/logging"
	"strings"

	"github.com/spf13/cobra"
)

// ConfigCmd represents the config command
var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage gcluster job configuration.",
	Long:  `Manage persistent configuration for gcluster job commands, such as default project, cluster, and location.`,
	// The config command should not check for the global flags to be already set.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key=value...]",
	Short: "Set configuration properties.",
	Long: `Set persistent configuration properties.
Supported keys:
  project   - Google Cloud Project ID
  cluster   - GKE Cluster Name
  location  - GKE Cluster Location (region or zone)


You can optionally provide multiple key=value pairs to set multiple values at once.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := loadContext()

		isOldFormatKey := false
		if len(args) > 0 {
			k := strings.ToLower(args[0])
			isOldFormatKey = (k == "project" || k == "cluster" || k == "location")
		}

		isSuspiciousValue := false
		if len(args) == 2 {
			valLower := strings.ToLower(args[1])
			isSuspiciousValue = strings.HasPrefix(valLower, "project=") || strings.HasPrefix(valLower, "cluster=") || strings.HasPrefix(valLower, "location=")
		}

		if len(args) == 2 && !strings.Contains(args[0], "=") && isOldFormatKey && !isSuspiciousValue {
			// backwards compatible mode for exactly 2 arguments (e.g. set project my-project)
			key := strings.ToLower(args[0])
			value := args[1]
			if err := setContextField(&ctx, key, value); err != nil {
				return err
			}
		} else {
			for _, arg := range args {
				key, value, found := strings.Cut(arg, "=")
				if !found {
					return fmt.Errorf("invalid argument format '%s', expected key=value", arg)
				}
				key = strings.ToLower(key)
				if err := setContextField(&ctx, key, value); err != nil {
					return err
				}
			}
		}

		if err := saveContext(ctx); err != nil {
			return err
		}
		logging.Info("Configuration updated successfully.")
		return nil
	},
}

func setContextField(ctx *Context, key, value string) error {
	switch key {
	case "project":
		ctx.ProjectID = value
	case "cluster":
		ctx.ClusterName = value
	case "location":
		ctx.Location = value
	default:
		return fmt.Errorf("invalid configuration key: %s. Supported keys: project, cluster, location", key)
	}
	return nil
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration properties.",
	Long:  `List all persistent configuration properties.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := loadContext()
		fmt.Fprintln(cmd.OutOrStdout(), "Current Configuration:")
		fmt.Fprintf(cmd.OutOrStdout(), "  project:  %s\n", ctx.ProjectID)
		fmt.Fprintf(cmd.OutOrStdout(), "  cluster:  %s\n", ctx.ClusterName)
		fmt.Fprintf(cmd.OutOrStdout(), "  location: %s\n", ctx.Location)
		return nil
	},
}

func init() {
	ConfigCmd.AddCommand(configSetCmd)
	ConfigCmd.AddCommand(configListCmd)
}
