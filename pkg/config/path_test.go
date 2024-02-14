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

	"github.com/zclconf/go-cty/cty"
)

func TestPath(t *testing.T) {
	type test struct {
		p    Path
		want string
	}
	r := Root
	m := r.Groups.At(3).Modules.At(1)
	tests := []test{
		{r, ""},
		{r.BlueprintName, "blueprint_name"},
		{r.GhpcVersion, "ghpc_version"},
		{r.Validators, "validators"},
		{r.ValidationLevel, "validation_level"},
		{r.Vars, "vars"},
		{r.Groups, "deployment_groups"},
		{r.Backend, "terraform_backend_defaults"},

		{r.Validators.At(2), "validators[2]"},
		{r.Validators.At(2).Validator, "validators[2].validator"},
		{r.Validators.At(2).Skip, "validators[2].skip"},
		{r.Validators.At(2).Inputs, "validators[2].inputs"},
		{r.Validators.At(2).Inputs.Dot("zebra"), "validators[2].inputs.zebra"},

		{r.Vars.Dot("red"), "vars.red"},
		{r.Vars.Dot("red").Cty(cty.Path{}), "vars.red"},
		{r.Vars.Dot("red").Cty(cty.Path{}.IndexInt(6)), "vars.red[6]"},
		{r.Vars.Dot("red").Cty(cty.Path{}.IndexInt(6).GetAttr("silver")), "vars.red[6].silver"},
		{r.Vars.Dot("red").Cty(cty.Path{}.IndexInt(6).IndexString("silver")), "vars.red[6].silver"},
		{r.Vars.Dot("red").Cty(cty.Path{}.IndexInt(6).Index(cty.True)), "vars.red[6]"}, // trim last piece as invalid

		{r.Groups.At(3), "deployment_groups[3]"},
		{r.Groups.At(3).Name, "deployment_groups[3].group"},
		{r.Groups.At(3).Backend, "deployment_groups[3].terraform_backend"},
		{r.Groups.At(3).Modules, "deployment_groups[3].modules"},
		{r.Groups.At(3).Modules.At(1), "deployment_groups[3].modules[1]"},
		// m := r.Groups.At(3).Modules.At(1)
		{m.Source, "deployment_groups[3].modules[1].source"},
		{m.ID, "deployment_groups[3].modules[1].id"},
		{m.Kind, "deployment_groups[3].modules[1].kind"},
		{m.Use, "deployment_groups[3].modules[1].use"},
		{m.Use.At(6), "deployment_groups[3].modules[1].use[6]"},
		{m.Outputs, "deployment_groups[3].modules[1].outputs"},
		{m.Outputs.At(2), "deployment_groups[3].modules[1].outputs[2]"},
		{m.Outputs.At(2).Name, "deployment_groups[3].modules[1].outputs[2].name"},
		{m.Outputs.At(2).Description, "deployment_groups[3].modules[1].outputs[2].description"},
		{m.Outputs.At(2).Sensitive, "deployment_groups[3].modules[1].outputs[2].sensitive"},
		{m.Settings, "deployment_groups[3].modules[1].settings"},
		{m.Settings.Dot("lime"), "deployment_groups[3].modules[1].settings.lime"},

		{r.Backend.Type, "terraform_backend_defaults.type"},
		{r.Backend.Configuration, "terraform_backend_defaults.configuration"},
		{r.Backend.Configuration.Dot("goo"), "terraform_backend_defaults.configuration.goo"},

		{internalPath, "__internal_path__"},
		{internalPath.Dot("a"), "__internal_path__.a"},
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

func TestPathParent(t *testing.T) {
	type test struct {
		p    Path
		want Path
	}
	r := Root
	cp := cty.Path{} // empty cty.Path
	tests := []test{
		{r, nil},
		{r.Groups, r},
		{r.Groups.At(3), r.Groups},
		{r.Groups.At(3).Modules, r.Groups.At(3)},
		{r.Vars.Dot("red"), r.Vars},
		{r.Vars.Dot("red").Cty(cp), r.Vars},
		{r.Vars.Dot("red").Cty(cp.IndexInt(6)), r.Vars.Dot("red")},
		{r.Vars.Dot("red").Cty(cp.IndexInt(6).IndexString("gg")), r.Vars.Dot("red").Cty(cp.IndexInt(6))},
		{r.Vars.Dot("red").Cty(cp.IndexInt(6).IndexString("gg").Index(cty.True)), r.Vars.Dot("red").Cty(cp.IndexInt(6))},
		{internalPath, nil},
		{internalPath.Dot("gold"), internalPath},
	}
	for _, tc := range tests {
		t.Run(tc.p.String(), func(t *testing.T) {
			got := tc.p.Parent()
			if (got == nil) || (tc.want == nil) {
				if got != tc.want {
					t.Errorf("\ngot : %#v\nwant: %#v", got, tc.want)
				}
				return
			}
			if got.String() != tc.want.String() {
				t.Errorf("\ngot : %q\nwant: %q", got.String(), tc.want.String())
			}
		})
	}
}
