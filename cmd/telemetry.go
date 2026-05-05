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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(telemetryCmd)
}

var telemetryCmd = &cobra.Command{
	Use:   "telemetry [on|off]",
	Short: "Enable or disable telemetry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var enabled bool

		switch strings.ToLower(args[0]) {
		case "on", "true", "yes", "enable":
			enabled = true
			logging.Info("Telemetry has been turned on.")
		case "off", "false", "no", "disable":
			enabled = false
			logging.Info("Telemetry has been turned off.")
		default:
			return fmt.Errorf("invalid argument %q: use 'on' or 'off'", args[0])
		}

		if err := config.SetTelemetry(enabled); err != nil {
			return err
		}

		return nil
	},
}
