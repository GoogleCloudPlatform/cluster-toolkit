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
	"hpc-toolkit/pkg/sourcereader"
	"hpc-toolkit/pkg/validators"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// validate is the top-level function for running the validation suite.
func (bc BlueprintConfig) validate() {
	if err := bc.validateValidators(); err != nil {
		// for now, warn that validators have failed
		log.Print(err)
	}
	if err := bc.validateVars(); err != nil {
		log.Fatal(err)
	}
	if err := bc.validateResources(); err != nil {
		log.Fatal(err)
	}
	if err := bc.validateResourceSettings(); err != nil {
		log.Fatal(err)
	}
}

// performs validation of global variables
// TODO: does not yet substitute/resolve variable values properly!
func (bc BlueprintConfig) validateValidators() error {
	var errored bool
	allValidators := getValidators()

	for _, validator := range bc.Config.Validators {
		if f, ok := allValidators[validator.Validator]; ok {
			val := f(validator)
			if val != nil {
				errored = true
				log.Print(val)
			}
		} else {
			errored = true
			log.Printf("%s is not an implemented validator", validator.Validator)
		}
	}

	if errored {
		log.Print("please confirm existence of resources and that credentials being used have access")
		return fmt.Errorf("at least one validator failed")
	}
	return nil
}

// validateVars checks the global variables for viable types
func (bc BlueprintConfig) validateVars() error {
	vars := bc.Config.Vars
	nilErr := "global variable %s was not set"

	// Check for project_id
	if _, ok := vars["project_id"]; !ok {
		log.Println("WARNING: No project_id in global variables")
	}

	// Check type of labels (if they are defined)
	if labels, ok := vars["labels"]; ok {
		if _, ok := labels.(map[string]interface{}); !ok {
			return errors.New("vars.labels must be a map")
		}
	}

	// Check for any nil values
	for key, val := range vars {
		if val == nil {
			return fmt.Errorf(nilErr, key)
		}
	}

	return nil
}

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

func validateOutputs(res Resource, resInfo resreader.ResourceInfo) error {

	// Only get the map if needed
	var outputsMap map[string]resreader.VarInfo
	if len(res.Outputs) > 0 {
		outputsMap = resInfo.GetOutputsAsMap()
	}

	// Ensure output exists in the underlying resource
	for _, output := range res.Outputs {
		if _, ok := outputsMap[output]; !ok {
			return fmt.Errorf("%s, resource: %s output: %s",
				errorMessages["invalidOutput"], res.ID, output)
		}
	}
	return nil
}

// validateResources ensures parameters set in resources are set correctly.
func (bc BlueprintConfig) validateResources() error {
	for _, grp := range bc.Config.ResourceGroups {
		for _, res := range grp.Resources {
			if err := validateResource(res); err != nil {
				return err
			}
			resInfo := bc.ResourcesInfo[grp.Name][res.Source]
			if err := validateOutputs(res, resInfo); err != nil {
				return err
			}
		}
	}
	return nil
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
func (bc BlueprintConfig) validateResourceSettings() error {
	for _, grp := range bc.Config.ResourceGroups {
		for _, res := range grp.Resources {
			reader := sourcereader.Factory(res.Source)
			info, err := reader.GetResourceInfo(res.Source, res.Kind)
			if err != nil {
				errStr := "failed to get info for resource at %s while validating resource settings"
				return errors.Wrapf(err, errStr, res.Source)
			}
			if err = validateSettings(res, info); err != nil {
				errStr := "found an issue while validating settings for resource at %s"
				return errors.Wrapf(err, errStr, res.Source)
			}
		}
	}
	return nil
}

func getValidators() map[string]func(validatorConfig) error {
	allValidators := map[string]func(validatorConfig) error{
		"test_project_exists": testProjectExists,
		"test_region_exists":  testRegionExists,
		"test_zone_exists":    testZoneExists,
		"test_zone_in_region": testZoneInRegion,
	}
	return allValidators
}

func testProjectExists(validator validatorConfig) error {
	err := validators.TestProjectExists(validator.ProjectID)
	if err == nil {
		log.Printf("projectID %s exists", validator.ProjectID)
	}
	return err
}

func testRegionExists(validator validatorConfig) error {
	err := validators.TestRegionExists(validator.ProjectID, validator.Region)
	if err == nil {
		log.Printf("region %s exists", validator.Region)
	}
	return err
}

func testZoneExists(validator validatorConfig) error {
	err := validators.TestZoneExists(validator.ProjectID, validator.Zone)
	if err == nil {
		log.Printf("zone %s exists", validator.Zone)
	}
	return err
}

func testZoneInRegion(validator validatorConfig) error {
	err := validators.TestZoneInRegion(validator.ProjectID, validator.Zone, validator.Region)
	if err == nil {
		log.Printf("zone %s is in region %s", validator.Zone, validator.Region)
	}
	return err
}
