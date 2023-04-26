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
	"hpc-toolkit/pkg/modulewriter"
	"os"
	"path"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

// GetDeploymentKinds performs a basic sanity check of metadata file and returns
// the module kinds for the deployment
func GetDeploymentKinds(metadataFile string, deploymentRoot string) (map[string]config.ModuleKind, error) {
	md, err := loadMetadata(metadataFile)
	if err != nil {
		return nil, err
	}

	groupKinds := make(map[string]config.ModuleKind)
	for _, gm := range md {
		groupPath := path.Join(deploymentRoot, gm.Name)
		if isDir, _ := DirInfo(groupPath); !isDir {
			return nil, fmt.Errorf("improper deployment: %s is not a directory for group %s", groupPath, gm.Name)
		}
		groupKinds[gm.Name] = gm.Kind
	}

	return groupKinds, nil
}

func loadMetadata(metadataFile string) ([]modulewriter.GroupMetadata, error) {
	reader, err := os.Open(metadataFile)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var md modulewriter.DeploymentMetadata
	if err := decoder.Decode(&md); err != nil {
		return nil, err
	}
	return md.DeploymentMetadata, nil
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

// DirInfo reports if path is a directory and new files can be written in it
func DirInfo(path string) (isDir bool, isWritable bool) {
	p, err := os.Lstat(path)
	if err != nil {
		return false, false
	}

	isDir = p.Mode().IsDir()
	isWritable = unix.Access(path, unix.W_OK|unix.X_OK) == nil

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
