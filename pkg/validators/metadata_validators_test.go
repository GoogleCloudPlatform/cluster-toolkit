// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package validators

import (
    "hpc-toolkit/pkg/config"
    "hpc-toolkit/pkg/modulereader"
    "testing"

    "github.com/zclconf/go-cty/cty"
)

func TestRegexValidator(t *testing.T) {
    // Fake blueprint for testing
    bp := config.Blueprint{
        BlueprintName: "test-bp",
        Groups: []config.Group{
            {
                Name: "primary",
                Modules: []config.Module{
                    {
                        ID:     "test-module",
                        Source: "test/module",
                        Settings: config.NewDict(map[string]cty.Value{
                            "name": cty.StringVal("Invalid-Name"),
                        }),
                    },
                },
            },
        },
    }

    // Validation rule from metadata.yaml
    rule := modulereader.ValidationRule{
        Validator:    "regex",
        ErrorMessage: "'name' must be lowercase and start with a letter.",
        Inputs: map[string]interface{}{
            "vars":    []interface{}{"name"},
            "pattern": "^[a-z]([-a-z0-9]*[a-z0-9])?$",
        },
    }

    validator := RegexValidator{}
    err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
    if err == nil {
        t.Errorf("Expected validation error, but got nil")
    }

    expectedError := "deployment_groups[0].modules[0].settings.name: 'name' must be lowercase and start with a letter."
    if err.Error() != expectedError {
        t.Errorf("Expected error message '%s', but got '%s'", expectedError, err.Error())
    }
}
