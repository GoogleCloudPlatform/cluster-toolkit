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
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
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

// return a map from group names to a list of outputs that are needed by this group
func getIntergroupOutputNamesByGroup(thisGroup config.GroupName, dc config.DeploymentConfig) (map[config.GroupName][]string, error) {
	thisGroupIdx := dc.Config.GroupIndex(thisGroup)
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
	outputsByGroup := make(map[config.GroupName][]string)
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

func getIntergroupPackerSettings(dc config.DeploymentConfig, packerModule config.Module) config.Dict {
	nonIntergroupSettings := map[string]bool{}
	for setting, v := range packerModule.Settings.Items() {
		igcRefs := config.FindIntergroupReferences(v, packerModule, dc.Config)
		if len(igcRefs) == 0 {
			nonIntergroupSettings[setting] = true
		}
	}

	packerGroup := dc.Config.ModuleGroupOrDie(packerModule.ID)
	igcRefs := modulewriter.FindIntergroupVariables(packerGroup, dc.Config)
	packerModule = modulewriter.SubstituteIgcReferencesInModule(packerModule, igcRefs)
	packerSettings := packerModule.Settings
	for setting := range nonIntergroupSettings {
		packerSettings.Unset(setting)
	}
	return packerSettings
}

// ApplyChangesChoice prompts the user to decide whether they want to approve
// changes to cloud configuration, to stop execution of ghpc entirely, or to
// skip making the proposed changes and continue execution (in deploy command)
// only if the user responds with "y" or "yes" (case-insensitive)
func ApplyChangesChoice(c ProposedChanges) bool {
	log.Printf("Summary of proposed changes: %s", strings.TrimSpace(c.Summary))
	var userResponse string

	for {
		fmt.Print("Display full proposed changes, Apply proposed changes, Stop and exit, Continue without applying? [d,a,s,c]: ")

		_, err := fmt.Scanln(&userResponse)
		if err != nil {
			log.Fatal(err)
		}

		switch strings.ToLower(strings.TrimSpace(userResponse)) {
		case "a":
			return true
		case "c":
			return false
		case "d":
			fmt.Println(c.Full)
		case "s":
			log.Fatal("user chose to stop execution of ghpc rather than make proposed changes to infrastructure")
		}
	}
}
