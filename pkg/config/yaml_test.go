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
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
)

func TestYamlCtx(t *testing.T) {
	data := `            # line 1
# comment
blueprint_name: green

ghpc_version: apricot

validators:
- validator: clay
  inputs:
    spice: curry         # line 10
- validator: sand
  skip: true

validation_level: 9000

vars:
  red: ruby

deployment_groups:
- group: tiger           # line 20
  terraform_backend:
    type: yam
    configuration:
      carrot: rust
  kind: terraform
  modules:
  - id: tan
    source: oatmeal
    kind: terraform
    use: [mocha, coffee] # line 30
    outputs:
    - latte
    - name: hazelnut
      description: almond
      sensitive: false
    settings:
      dijon: pine

- group: crocodile
  modules:               # line 40
  - id: green
  - id: olive

terraform_backend_defaults:
  type: moss
`

	{ // Tests sanity check - data describes valid blueprint.
		decoder := yaml.NewDecoder(bytes.NewReader([]byte(data)))
		decoder.KnownFields(true)
		var bp Blueprint
		if err := decoder.Decode(&bp); err != nil {
			t.Fatal(err)
		}
	}

	type test struct {
		path Path
		want Pos
	}
	tests := []test{
		{Root, Pos{3, 1}},
		{Root.BlueprintName, Pos{3, 17}},
		{Root.GhpcVersion, Pos{5, 15}},
		{Root.Validators, Pos{8, 1}},
		{Root.Validators.At(0), Pos{8, 3}},
		{Root.Validators.At(0).Inputs, Pos{10, 5}},
		{Root.Validators.At(0).Inputs.Dot("spice"), Pos{10, 12}},
		{Root.Validators.At(0).Validator, Pos{8, 14}},
		{Root.Validators.At(1), Pos{11, 3}},
		{Root.Validators.At(1).Skip, Pos{12, 9}},
		{Root.Validators.At(1).Validator, Pos{11, 14}},
		{Root.ValidationLevel, Pos{14, 19}},
		{Root.Vars, Pos{17, 3}},
		{Root.Vars.Dot("red"), Pos{17, 8}},
		{Root.Groups, Pos{20, 1}},
		{Root.Groups.At(0), Pos{20, 3}},
		{Root.Groups.At(0).Name, Pos{20, 10}},

		{Root.Groups.At(0).Backend, Pos{22, 5}},
		{Root.Groups.At(0).Backend.Type, Pos{22, 11}},
		{Root.Groups.At(0).Backend.Configuration, Pos{24, 7}},
		{Root.Groups.At(0).Backend.Configuration.Dot("carrot"), Pos{24, 15}},
		{Root.Groups.At(0).Kind, Pos{25, 9}},

		{Root.Groups.At(0).Modules, Pos{27, 3}},
		{Root.Groups.At(0).Modules.At(0), Pos{27, 5}},
		{Root.Groups.At(0).Modules.At(0).ID, Pos{27, 9}},
		{Root.Groups.At(0).Modules.At(0).Source, Pos{28, 13}},
		{Root.Groups.At(0).Modules.At(0).Kind, Pos{29, 11}},
		{Root.Groups.At(0).Modules.At(0).Use, Pos{30, 10}},
		{Root.Groups.At(0).Modules.At(0).Use.At(0), Pos{30, 11}},
		{Root.Groups.At(0).Modules.At(0).Use.At(1), Pos{30, 18}},
		{Root.Groups.At(0).Modules.At(0).Outputs, Pos{32, 5}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(0), Pos{32, 7}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(0).Name, Pos{32, 7}}, // synthetic
		{Root.Groups.At(0).Modules.At(0).Outputs.At(1), Pos{33, 7}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(1).Name, Pos{33, 13}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(1).Description, Pos{34, 20}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(1).Sensitive, Pos{35, 18}},
		{Root.Groups.At(0).Modules.At(0).Settings, Pos{37, 7}},
		{Root.Groups.At(0).Modules.At(0).Settings.Dot("dijon"), Pos{37, 14}},

		{Root.Groups.At(1), Pos{39, 3}},
		{Root.Groups.At(1).Name, Pos{39, 10}},
		{Root.Groups.At(1).Modules, Pos{41, 3}},
		{Root.Groups.At(1).Modules.At(0), Pos{41, 5}},
		{Root.Groups.At(1).Modules.At(0).ID, Pos{41, 9}},
		{Root.Groups.At(1).Modules.At(1), Pos{42, 5}},
		{Root.Groups.At(1).Modules.At(1).ID, Pos{42, 9}},

		{Root.Backend, Pos{45, 3}},
		{Root.Backend.Type, Pos{45, 9}},
	}

	ctx := NewYamlCtx([]byte(data))
	for _, tc := range tests {
		t.Run(tc.path.String(), func(t *testing.T) {
			got, ok := ctx.Pos(tc.path)
			if !ok {
				t.Errorf("%q not found", tc.path.String())
			} else if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}
