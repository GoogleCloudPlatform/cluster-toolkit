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
	"os"
	"path/filepath"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"
)

// GetDeploymentKinds returns the kind of each group in the deployment as a map;
// additionally it provides a mechanism for validating the deployment directory
// structure; for now, validation tests only existence of each directory
func GetDeploymentKinds(expandedBlueprintFile string) (map[string]config.ModuleKind, error) {
	dc, err := config.NewDeploymentConfig(expandedBlueprintFile)
	if err != nil {
		return nil, err
	}

	groupKinds := make(map[string]config.ModuleKind)
	for _, g := range dc.Config.DeploymentGroups {
		if g.Kind == config.UnknownKind {
			return nil, fmt.Errorf("improper deployment: group %s is of unknown kind", g.Name)
		}
		groupKinds[g.Name] = g.Kind
	}
	return groupKinds, nil
}

// ValidateDeploymentDirectory ensures that the deployment directory structure
// appears valid given a mapping of group names to module kinds
// TODO: verify kind fully by auto-detecting type from group directory
func ValidateDeploymentDirectory(kinds map[string]config.ModuleKind, deploymentRoot string) error {
	for group := range kinds {
		groupPath := filepath.Join(deploymentRoot, group)
		if isDir, _ := DirInfo(groupPath); !isDir {
			return fmt.Errorf("improper deployment: %s is not a directory for group %s", groupPath, group)
		}
	}
	return nil
}

// return a map from group names to a list of outputs that are needed by this group
func getIntergroupOutputNamesByGroup(thisGroup string, expandedBlueprintFile string) (map[string][]string, error) {
	dc, err := config.NewDeploymentConfig(expandedBlueprintFile)
	if err != nil {
		return nil, err
	}

	thisGroupIdx := slices.IndexFunc(dc.Config.DeploymentGroups, func(g config.DeploymentGroup) bool { return g.Name == thisGroup })
	if thisGroupIdx == -1 {
		return nil, fmt.Errorf("this group wasn't found in the deployment metadata")
	}
	if thisGroupIdx == 0 {
		return nil, nil
	}

	thisIntergroupRefs := dc.Config.DeploymentGroups[thisGroupIdx].FindAllIntergroupReferences(dc.Config)
	thisIntergroupInputNames := make([]string, len(thisIntergroupRefs))
	for i, ref := range thisIntergroupRefs {
		thisIntergroupInputNames[i] = config.AutomaticOutputName(ref.Name, ref.Module)
	}
	outputsByGroup := make(map[string][]string)
	for _, g := range dc.Config.DeploymentGroups[:thisGroupIdx] {
		outputsByGroup[g.Name] = intersection(thisIntergroupInputNames, g.OutputNames())
	}
	return outputsByGroup, nil
}

// return sorted list of elements common to s1 and s2
func intersection(s1 []string, s2 []string) []string {
	count := make(map[string]int)

	for _, v := range s1 {
		count[v]++
	}

	foundInBoth := map[string]bool{}
	for _, v := range s2 {
		if count[v] > 0 {
			foundInBoth[v] = true
		}
	}
	is := maps.Keys(foundInBoth)
	slices.Sort(is)
	return is
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

func mergeMapsWithoutLoss[K comparable, V any](m1 map[K]V, m2 map[K]V) {
	expectedLength := len(m1) + len(m2)
	maps.Copy(m1, m2)
	if len(m1) != expectedLength {
		panic(fmt.Errorf("unexpected key collision in maps"))
	}
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
