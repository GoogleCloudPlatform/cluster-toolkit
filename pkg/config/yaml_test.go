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

	exp := map[string]Pos{
		"":               {3, 1},
		"blueprint_name": {3, 17},
		"ghpc_version":   {5, 15},

		"validators":                 {8, 1},
		"validators[0]":              {8, 3},
		"validators[0].inputs":       {10, 5},
		"validators[0].inputs.spice": {10, 12},
		"validators[0].validator":    {8, 14},
		"validators[1]":              {11, 3},
		"validators[1].skip":         {12, 9},
		"validators[1].validator":    {11, 14},

		"validation_level": {14, 19},

		"vars":     {17, 3},
		"vars.red": {17, 8},

		"deployment_groups":          {20, 1},
		"deployment_groups[0]":       {20, 3},
		"deployment_groups[0].group": {20, 10},

		"deployment_groups[0].terraform_backend":                      {22, 5},
		"deployment_groups[0].terraform_backend.type":                 {22, 11},
		"deployment_groups[0].terraform_backend.configuration":        {24, 7},
		"deployment_groups[0].terraform_backend.configuration.carrot": {24, 15},
		"deployment_groups[0].kind":                                   {25, 9},

		"deployment_groups[0].modules":                           {27, 3},
		"deployment_groups[0].modules[0]":                        {27, 5},
		"deployment_groups[0].modules[0].id":                     {27, 9},
		"deployment_groups[0].modules[0].source":                 {28, 13},
		"deployment_groups[0].modules[0].kind":                   {29, 11},
		"deployment_groups[0].modules[0].use":                    {30, 10},
		"deployment_groups[0].modules[0].use[0]":                 {30, 11},
		"deployment_groups[0].modules[0].use[1]":                 {30, 18},
		"deployment_groups[0].modules[0].outputs":                {32, 5},
		"deployment_groups[0].modules[0].outputs[0]":             {32, 7},
		"deployment_groups[0].modules[0].outputs[0].name":        {32, 7}, // synthetic
		"deployment_groups[0].modules[0].outputs[1]":             {33, 7},
		"deployment_groups[0].modules[0].outputs[1].name":        {33, 13},
		"deployment_groups[0].modules[0].outputs[1].description": {34, 20},
		"deployment_groups[0].modules[0].outputs[1].sensitive":   {35, 18},
		"deployment_groups[0].modules[0].settings":               {37, 7},
		"deployment_groups[0].modules[0].settings.dijon":         {37, 14},

		"deployment_groups[1]":               {39, 3},
		"deployment_groups[1].group":         {39, 10},
		"deployment_groups[1].modules":       {41, 3},
		"deployment_groups[1].modules[0]":    {41, 5},
		"deployment_groups[1].modules[0].id": {41, 9},
		"deployment_groups[1].modules[1]":    {42, 5},
		"deployment_groups[1].modules[1].id": {42, 9},

		"terraform_backend_defaults":      {45, 3},
		"terraform_backend_defaults.type": {45, 9},
	}

	ctx := newYamlCtx([]byte(data))
	for path, pos := range exp {
		t.Run(path, func(t *testing.T) {
			got, ok := ctx.PathToPos[Path{path}]
			if !ok {
				t.Errorf("%q not found", path)
			} else if diff := cmp.Diff(pos, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}
