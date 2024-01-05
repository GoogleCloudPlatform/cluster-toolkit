// Copyright 2023 "Google LLC"
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

package validators

import (
	"fmt"
	"hpc-toolkit/pkg/config"

	"golang.org/x/exp/slices"
)

func testModuleNotUsed(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{}); err != nil {
		return err
	}
	errs := config.Errors{}
	bp.WalkModulesSafe(func(p config.ModulePath, m *config.Module) {
		ums := m.ListUnusedModules()
		for iu, u := range m.Use {
			if slices.Contains(ums, u) {
				errs.At(p.Use.At(iu), fmt.Errorf(unusedModuleMsg, m.ID, u))
			}
		}
	})
	return errs.OrNil()
}

func testDeploymentVariableNotUsed(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{}); err != nil {
		return err
	}
	errs := config.Errors{}
	for _, v := range bp.ListUnusedVariables() {
		errs.At(
			config.Root.Vars.Dot(v),
			fmt.Errorf("the variable %q was not used in this blueprint", v))
	}
	return errs.OrNil()
}
