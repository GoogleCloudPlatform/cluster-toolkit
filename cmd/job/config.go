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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// ConfigCmd represents the config command
var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage gcluster job configuration.",
	Long:  `Manage persistent configuration for gcluster job commands.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Modify defaults (Opens editor if key and value are omitted)",
	Long: `Allows quick updates to individual configuration defaults.
If run without any arguments, it automatically opens your default system text editor.`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return openInSystemEditor()
		}

		if len(args) == 1 {
			return fmt.Errorf("requires both a key and a value (e.g. gcluster job config set project my-project)")
		}

		ctx, err := loadContext()
		if err != nil {
			return err
		}

		key := strings.ToLower(args[0])
		val := args[1]

		if val == "" {
			return fmt.Errorf("configuration values cannot be empty")
		}

		switch key {
		case "project":
			ctx.ProjectID = val
		case "cluster":
			ctx.ClusterName = val
		case "location":
			ctx.Location = val
		default:
			return fmt.Errorf("unknown configuration key '%s', available configuration keys are: 'project', 'cluster', 'location'", key)
		}

		if err := saveContext(ctx); err != nil {
			return err
		}
		cmd.Printf("Updated default for '%s' to '%s'\n", key, val)
		return nil
	},
}

func createTempConfigFile(sourcePath string) (string, error) {
	tempFile, err := os.CreateTemp("", "gcluster-config-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tempFileName := tempFile.Name()

	fileData, err := os.ReadFile(sourcePath)
	if err == nil {
		if _, err := tempFile.Write(fileData); err != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempFileName)
			return "", fmt.Errorf("failed to write to temp file: %w", err)
		}
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFileName)
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}
	return tempFileName, nil
}

func runSystemEditor(editor, filePath string) error {
	editorParts := strings.Fields(editor)
	if len(editorParts) == 0 {
		return fmt.Errorf("editor command is empty")
	}
	args := append(editorParts[1:], filePath)
	c := exec.Command(editorParts[0], args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("failed to open system editor (%s): %w", editor, err)
	}
	return nil
}

func validateAndApplyConfig(tempFileName, targetFile string) error {
	tempFileData, err := os.ReadFile(tempFileName)
	if err != nil {
		return fmt.Errorf("failed to read temp file: %w", err)
	}

	var tempConfig Context
	trimmedData := strings.TrimSpace(string(tempFileData))
	if trimmedData == "" {
		tempFileData = []byte("{}")
	} else {
		decoder := json.NewDecoder(strings.NewReader(trimmedData))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&tempConfig); err != nil {
			return fmt.Errorf("config file contains structural errors or invalid JSON: %w", err)
		}
	}

	if err := os.WriteFile(targetFile, tempFileData, 0644); err != nil {
		return fmt.Errorf("failed to save config file: %w", err)
	}

	if tempConfig.ProjectID == "" || tempConfig.ClusterName == "" || tempConfig.Location == "" {
		fmt.Println("Note: Some configuration fields were left empty. You will need to provide them via command-line flags when submitting jobs.")
	}
	return nil
}

func openInSystemEditor() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("cannot open editor in a non-interactive terminal")
	}

	configFile, err := contextFilePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		if err := saveContext(Context{}); err != nil {
			return err
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	tempFileName, err := createTempConfigFile(configFile)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tempFileName)
	}()

	if err := runSystemEditor(editor, tempFileName); err != nil {
		return err
	}

	if err := validateAndApplyConfig(tempFileName, configFile); err != nil {
		return err
	}

	fmt.Println("Configuration file updated successfully.")
	return nil
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print current saved defaults",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadContext()
		if err != nil {
			return err
		}
		configFile, err := contextFilePath()
		if err != nil {
			return err
		}

		cmd.Printf("Configuration Path: %s\n\n", configFile)
		cmd.Printf("  %-15s %s\n", "KEY", "VALUE")
		cmd.Printf("  %-15s %s\n", "---", "-----")
		cmd.Printf("  %-15s %s\n", "project", ctx.ProjectID)
		cmd.Printf("  %-15s %s\n", "cluster", ctx.ClusterName)
		cmd.Printf("  %-15s %s\n", "location", ctx.Location)
		return nil
	},
}

func init() {
	ConfigCmd.AddCommand(configSetCmd)
	ConfigCmd.AddCommand(configShowCmd)
}
