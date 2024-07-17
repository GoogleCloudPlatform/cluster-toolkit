// Copyright 2022 Google LLC
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

package validators

import (
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

func projectError(p string) error {
	return config.HintError{
		Err: fmt.Errorf("project %q does not exist or your credentials do not have permission to access it", p),
		Hint: "It is possible the machine you are working on has not been authenticated.\n" +
			"Try to run `gcloud auth application-default login`",
	}
}

const regionError = "region %s is not available in project ID %s or your credentials do not have permission to access it"
const zoneError = "zone %s is not available in project ID %s or your credentials do not have permission to access it"
const zoneInRegionError = "zone %s is not in region %s in project ID %s or your credentials do not have permissions to access it"
const unusedModuleMsg = "module %q uses module %q, but matching setting and outputs were not found. This may be because the value is set explicitly or set by a prior used module"
const credentialsHint = "load application default credentials following instructions at https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/main/README.md#supplying-cloud-credentials-to-terraform"

var ErrNoDefaultCredentials = errors.New("could not find application default credentials")

func handleClientError(e error) error {
	if strings.Contains(e.Error(), "could not find default credentials") {
		return config.HintError{Hint: credentialsHint, Err: ErrNoDefaultCredentials}
	}
	return e
}

const (
	testApisEnabledName               = "test_apis_enabled"
	testProjectExistsName             = "test_project_exists"
	testRegionExistsName              = "test_region_exists"
	testZoneExistsName                = "test_zone_exists"
	testZoneInRegionName              = "test_zone_in_region"
	testModuleNotUsedName             = "test_module_not_used"
	testDeploymentVariableNotUsedName = "test_deployment_variable_not_used"
	testTfVersionForSlurmName         = "test_tf_version_for_slurm"
)

func implementations() map[string]func(config.Blueprint, config.Dict) error {
	return map[string]func(config.Blueprint, config.Dict) error{
		testApisEnabledName:               testApisEnabled,
		testProjectExistsName:             testProjectExists,
		testRegionExistsName:              testRegionExists,
		testZoneExistsName:                testZoneExists,
		testZoneInRegionName:              testZoneInRegion,
		testModuleNotUsedName:             testModuleNotUsed,
		testDeploymentVariableNotUsedName: testDeploymentVariableNotUsed,
		testTfVersionForSlurmName:         testTfVersionForSlurm,
	}
}

// ValidatorError is an error wrapper for errors that occurred during validation
type ValidatorError struct {
	Validator string
	Err       error
}

func (e ValidatorError) Unwrap() error {
	return e.Err
}

func (e ValidatorError) Error() string {
	return fmt.Sprintf("validator %q failed:\n%v", e.Validator, e.Err)
}

// Execute runs all validators on the blueprint
func Execute(bp config.Blueprint) error {
	if bp.ValidationLevel == config.ValidationIgnore {
		return nil
	}
	impl := implementations()
	errs := config.Errors{}
	for iv, v := range validators(bp) {
		p := config.Root.Validators.At(iv)
		if v.Skip {
			continue
		}

		f, ok := impl[v.Validator]
		if !ok {
			errs.At(p.Validator, fmt.Errorf("unknown validator %q", v.Validator))
			continue
		}

		inp, err := bp.EvalDict(v.Inputs)
		if err != nil {
			errs.At(p.Inputs, err)
			continue
		}

		if err := f(bp, inp); err != nil {
			errs.Add(ValidatorError{v.Validator, err})
			// do not bother running further validators if project ID could not be found
			if v.Validator == "test_project_exists" {
				break
			}
		}
	}
	return errs.OrNil()
}

func checkInputs(inputs config.Dict, required []string) error {
	errs := config.Errors{}
	for _, inp := range required {
		if !inputs.Has(inp) {
			errs.Add(fmt.Errorf("a required input %q was not provided", inp))
		}
	}

	if errs.Any() {
		return errs
	}

	// ensure that no extra inputs were provided by comparing length
	if len(required) != len(inputs.Items()) {
		errStr := "only %v inputs %s should be provided"
		return fmt.Errorf(errStr, len(required), required)
	}

	return nil
}

// Helper function to sure that all input values are strings.
func inputsAsStrings(inputs config.Dict) (map[string]string, error) {
	ms := map[string]string{}
	for k, v := range inputs.Items() {
		if v.Type() != cty.String {
			return nil, fmt.Errorf("validator inputs must be strings, %s is a %s", k, v.Type())
		}
		ms[k] = v.AsString()
	}
	return ms, nil
}

// Creates a list of default validators for the given blueprint,
// inspect the blueprint for global variables that exist and add an appropriate validators.
func defaults(bp config.Blueprint) []config.Validator {
	projectIDExists := bp.Vars.Has("project_id")
	projectRef := config.GlobalRef("project_id").AsValue()

	regionExists := bp.Vars.Has("region")
	regionRef := config.GlobalRef("region").AsValue()

	zoneExists := bp.Vars.Has("zone")
	zoneRef := config.GlobalRef("zone").AsValue()

	defaults := []config.Validator{
		{Validator: testModuleNotUsedName},
		{Validator: testDeploymentVariableNotUsedName},
		{Validator: testTfVersionForSlurmName}}

	// always add the project ID validator before subsequent validators that can
	// only succeed if credentials can access the project. If the project ID
	// validator fails, all remaining validators are not executed.
	if projectIDExists {
		inputs := config.Dict{}.With("project_id", projectRef)
		defaults = append(defaults, config.Validator{
			Validator: testProjectExistsName,
			Inputs:    inputs,
		}, config.Validator{
			Validator: testApisEnabledName,
			Inputs:    inputs,
		},
		)
	}

	if projectIDExists && regionExists {
		defaults = append(defaults, config.Validator{
			Validator: testRegionExistsName,
			Inputs: config.NewDict(map[string]cty.Value{
				"project_id": projectRef,
				"region":     regionRef,
			},
			)})
	}

	if projectIDExists && zoneExists {
		defaults = append(defaults, config.Validator{
			Validator: testZoneExistsName,
			Inputs: config.NewDict(map[string]cty.Value{
				"project_id": projectRef,
				"zone":       zoneRef,
			}),
		})
	}

	if projectIDExists && regionExists && zoneExists {
		defaults = append(defaults, config.Validator{
			Validator: testZoneInRegionName,
			Inputs: config.NewDict(map[string]cty.Value{
				"project_id": projectRef,
				"region":     regionRef,
				"zone":       zoneRef,
			}),
		})
	}
	return defaults
}

// Returns a list of validators for the given blueprint with any default validators appended.
func validators(bp config.Blueprint) []config.Validator {
	used := map[string]bool{}
	for _, v := range bp.Validators {
		used[v.Validator] = true
	}
	vs := append([]config.Validator{}, bp.Validators...) // clone
	for _, v := range defaults(bp) {
		if !used[v.Validator] {
			vs = append(vs, v)
		}
	}
	return vs
}
