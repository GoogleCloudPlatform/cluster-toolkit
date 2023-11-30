/**
 * Copyright 2023 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package shell

import (
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ProposedChanges provides summary and full description of proposed changes
// to cloud infrastructure
type ProposedChanges struct {
	Summary string
	Full    string
}

// ValidateDeploymentDirectory ensures that the deployment directory structure
// appears valid given a mapping of group names to module kinds
// TODO: verify kind fully by auto-detecting type from group directory
func ValidateDeploymentDirectory(groups []config.DeploymentGroup, deploymentRoot string) error {
	for _, group := range groups {
		groupPath := filepath.Join(deploymentRoot, string(group.Name))
		if isDir, _ := DirInfo(groupPath); !isDir {
			return fmt.Errorf("improper deployment: %s is not a directory for group %s", groupPath, group.Name)
		}
	}
	return nil
}

func intersectMapKeys[K comparable, T any](s []K, m map[K]T) map[K]T {
	intersection := make(map[K]T)
	for _, e := range s {
		if val, ok := m[e]; ok {
			intersection[e] = val
		}
	}
	return intersection
}

func mergeMapsWithoutLoss[K comparable, V any](to map[K]V, from map[K]V) error {
	for k, v := range from {
		if _, ok := to[k]; ok {
			return fmt.Errorf("duplicate key %v", k)
		}
		to[k] = v
	}
	return nil
}

// DirInfo reports if path is a directory and new files can be written in it
func DirInfo(path string) (isDir bool, isWritable bool) {
	p, err := os.Lstat(path)
	if err != nil {
		return false, false
	}

	isDir = p.Mode().IsDir()
	isWritable = unix.Access(path, unix.W_OK|unix.R_OK|unix.X_OK) == nil

	return isDir, isWritable
}

// CheckWritableDir errors unless path is a directory we can write to
func CheckWritableDir(path string) error {
	if path == "" {
		return nil
	}
	if isDir, isWritable := DirInfo(path); !(isDir && isWritable) {
		return fmt.Errorf("%s must be a writable directory", path)
	}
	return nil
}

// ApplyChangesChoice prompts the user to decide whether they want to approve
// changes to cloud configuration, to stop execution of ghpc entirely, or to
// skip making the proposed changes and continue execution (in deploy command)
// only if the user responds with "y" or "yes" (case-insensitive)
func ApplyChangesChoice(c ProposedChanges) bool {
	logging.Info("Summary of proposed changes: %s", strings.TrimSpace(c.Summary))
	var userResponse string

	for {
		fmt.Print(`(D)isplay full proposed changes,
(A)pply proposed changes,
(S)top and exit,
(C)ontinue without applying
Please select an option [d,a,s,c]: `)

		_, err := fmt.Scanln(&userResponse)
		if err != nil {
			logging.Fatal("%v", err)
		}

		switch strings.ToLower(strings.TrimSpace(userResponse)) {
		case "a":
			return true
		case "c":
			return false
		case "d":
			fmt.Println(c.Full)
		case "s":
			logging.Fatal("user chose to stop execution of ghpc rather than make proposed changes to infrastructure")
		}
	}
}
