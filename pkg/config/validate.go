/**
 * Copyright 2021 Google LLC
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

package config

import (
	"fmt"
	"log"
	"regexp"

	"hpc-toolkit/pkg/resreader"

	"gopkg.in/yaml.v2"
)

func resource2String(c Resource) string {
	cBytes, _ := yaml.Marshal(&c)
	return string(cBytes)
}

func validateResource(c Resource) error {
	if c.ID == "" {
		return fmt.Errorf("%s\n%s", errorMessages["emptyID"], resource2String(c))
	}
	if c.Source == "" {
		return fmt.Errorf("%s\n%s", errorMessages["emptySource"], resource2String(c))
	}
	if !resreader.IsValidKind(c.Kind) {
		return fmt.Errorf("%s\n%s", errorMessages["wrongKind"], resource2String(c))
	}
	return nil
}

func hasIllegalChars(name string) bool {
	return !regexp.MustCompile(`^[\w\+]+(\s*)[\w-\+\.]+$`).MatchString(name)
}

// validateResources ensures parameters set in resources are set correctly.
func (bc BlueprintConfig) validateResources() {
	for _, grp := range bc.Config.ResourceGroups {
		for _, res := range grp.Resources {
			err := validateResource(res)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

type resourceVariables struct {
	Inputs  map[string]bool
	Outputs map[string]bool
}

func validateSettings(
	res Resource,
	info resreader.ResourceInfo) error {

	var cVars = resourceVariables{
		Inputs:  map[string]bool{},
		Outputs: map[string]bool{},
	}

	for _, input := range info.Inputs {
		cVars.Inputs[input.Name] = input.Required
	}
	// Make sure we only define variables that exist
	for k := range res.Settings {
		if _, ok := cVars.Inputs[k]; !ok {
			return fmt.Errorf("%s: Resource.ID: %s Setting: %s",
				errorMessages["extraSetting"], res.ID, k)
		}
	}
	return nil
}

// validateResourceSettings verifies that no additional settings are provided
// that don't have a counterpart variable in the resource.
func (bc BlueprintConfig) validateResourceSettings() {
	for _, grp := range bc.Config.ResourceGroups {
		for _, res := range grp.Resources {
			reader := resreader.Factory(res.Kind)
			info, err := reader.GetInfo(res.Source)
			if err != nil {
				log.Fatalf(
					"failed to get info for resource at %s while validating resource settings: %e",
					res.Source, err)
			}
			if err = validateSettings(res, info); err != nil {
				log.Fatalf(
					"found an issue while validating settings for resource at %s: %e",
					res.Source, err)
			}
		}
	}
}
