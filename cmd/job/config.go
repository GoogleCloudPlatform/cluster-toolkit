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
	"os/exec"
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
  
Can be used with --from-gcloud to infer settings from the active kubectl context.
You can optionally provide key=value pairs to override the inferred context, or to set multiple values at once.`,
	Args: func(cmd *cobra.Command, args []string) error {
		fromGcloud, err := cmd.Flags().GetBool("from-gcloud")
		if err == nil && fromGcloud {
			return cobra.MinimumNArgs(0)(cmd, args)
		}
		return cobra.MinimumNArgs(1)(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := loadContext()
		fromGcloud, err := cmd.Flags().GetBool("from-gcloud")
		if err != nil {
			return err
		}

		if fromGcloud {
			cmd := exec.Command("kubectl", "config", "current-context")
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				if stderr.Len() > 0 {
					return fmt.Errorf("could not infer context from kubectl: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
				}
				return fmt.Errorf("could not infer context from kubectl: %w", err)
			}
			contextStr := strings.TrimSpace(string(out))
			parts := strings.Split(contextStr, "_")
			if len(parts) == 4 && parts[0] == "gke" {
				ctx.ProjectID = parts[1]
				ctx.Location = parts[2]
				ctx.ClusterName = parts[3]
				logging.Info("Inferred context from gcloud: project=%s, location=%s, cluster=%s", ctx.ProjectID, ctx.Location, ctx.ClusterName)
			} else {
				return fmt.Errorf("active kubectl context '%s' does not match expected GKE format (gke_project_location_cluster)", contextStr)
			}
		}

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
	configSetCmd.Flags().Bool("from-gcloud", false, "Infer project, location, and cluster from the active kubectl context.")
	ConfigCmd.AddCommand(configSetCmd)
	ConfigCmd.AddCommand(configListCmd)
}
