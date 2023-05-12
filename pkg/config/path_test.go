// Copyright 2023 Google LLC
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

package config

import (
	"testing"
)

func TestPathString(t *testing.T) {
	type test struct {
		p    Path
		want string
	}
	r := rootPath()
	m := r.DeploymentGroups().At(3).Modules().At(1)
	tests := []test{
		{r, ""},
		{r.BlueprintName(), "blueprint_name"},
		{r.GhpcVersion(), "ghpc_version"},
		{r.Validators(), "validators"},
		{r.ValidationLevel(), "validation_level"},
		{r.Vars(), "vars"},
		{r.DeploymentGroups(), "deployment_groups"},
		{r.TerraformBackendDefaults(), "terraform_backend_defaults"},

		{r.Validators().At(2), "validators[2]"},
		{r.Validators().At(2).Validator(), "validators[2].validator"},
		{r.Validators().At(2).Skip(), "validators[2].skip"},
		{r.Validators().At(2).Inputs(), "validators[2].inputs"},
		{r.Validators().At(2).Inputs().At("zebra"), "validators[2].inputs[zebra]"},

		{r.Vars().At("red"), "vars[red]"},

		{r.DeploymentGroups().At(3), "deployment_groups[3]"},
		{r.DeploymentGroups().At(3).Name(), "deployment_groups[3].name"},
		{r.DeploymentGroups().At(3).Kind(), "deployment_groups[3].kind"},
		{r.DeploymentGroups().At(3).TerraformBackend(), "deployment_groups[3].terraform_backend"},
		{r.DeploymentGroups().At(3).Modules(), "deployment_groups[3].modules"},
		{r.DeploymentGroups().At(3).Modules().At(1), "deployment_groups[3].modules[1]"},
		// m := r.DeploymentGroups().At(3).Modules().At(1)
		{m.Source(), "deployment_groups[3].modules[1].source"},
		{m.ID(), "deployment_groups[3].modules[1].id"},
		{m.Kind(), "deployment_groups[3].modules[1].kind"},
		{m.Use(), "deployment_groups[3].modules[1].use"},
		{m.Use().At(6), "deployment_groups[3].modules[1].use[6]"},
		{m.Outputs(), "deployment_groups[3].modules[1].outputs"},
		{m.Outputs().At(2), "deployment_groups[3].modules[1].outputs[2]"},
		{m.Outputs().At(2).Name(), "deployment_groups[3].modules[1].outputs[2].name"},
		{m.Outputs().At(2).Description(), "deployment_groups[3].modules[1].outputs[2].description"},
		{m.Outputs().At(2).Sensitive(), "deployment_groups[3].modules[1].outputs[2].sensitive"},
		{m.Settings(), "deployment_groups[3].modules[1].settings"},
		{m.Settings().At("lime"), "deployment_groups[3].modules[1].settings[lime]"},

		{r.TerraformBackendDefaults().Type(), "terraform_backend_defaults.type"},
		{r.TerraformBackendDefaults().Configuration(), "terraform_backend_defaults.configuration"},
		{r.TerraformBackendDefaults().Configuration().At("goo"), "terraform_backend_defaults.configuration[goo]"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.p.String()
			if got != tc.want {
				t.Errorf("\ngot : %q\nwant: %q", got, tc.want)
			}
		})
	}
}
