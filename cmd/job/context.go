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

package job

import (
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"os"
	"path/filepath"
)

func contextFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	stateDir := filepath.Join(homeDir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return "", fmt.Errorf("could not create state directory %s: %w", stateDir, err)
	}
	return filepath.Join(stateDir, contextFileName), nil
}

func loadContext() Context {
	filePath, err := contextFilePath()
	if err != nil {
		logging.Error("Failed to get context file path: %v", err)
		return Context{}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.Error("Failed to read context from %s: %v", filePath, err)
		}
		return Context{}
	}

	var ctx Context
	if err := json.Unmarshal(data, &ctx); err != nil {
		logging.Error("Failed to unmarshal context from %s: %v", filePath, err)
		return Context{}
	}
	return ctx
}

func saveContext(ctx Context) error {
	filePath, err := contextFilePath()
	if err != nil {
		return fmt.Errorf("failed to get context file path for saving: %w", err)
	}

	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal context: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write context to %s: %w", filePath, err)
	}
	logging.Info("CLI context saved to %s", filePath)
	return nil
}
