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
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/exp/maps"
)

func TestYamlCtx(t *testing.T) {
	data := `
# comment
blueprint_name: green

ghpc_version: apricot

validators:
- validator: clay
  inputs:
  spice: curry
  herb: [basil, sage]
- validator: sand
  skip: true

validation_level: 9000

vars:
  red: ruby
  blue: [blush, candy]
  berry: 
    scarlet:
    - blood: [jam, wine]
    - sangria:
        merlot: [cabernet, pinot]

deployment_groups:
- group: tiger
  terraform_backend:
    type: yam
    configuration:
      carrot: rust
      squash: [amber]
  kind: ginger
  modules:
  - id: tan
    source: oatmeal
    kind: fawn
    use: [mocha, coffee]
    outputs:
    - latte
    - capppuccino:
      name: hazelnut
      description: almond
      sensitive: false
    settings:
      dijon: pine
      seaweed: [kelp, nori]

- group: crocodile
  modules:
  - id: green
  - id: olive

terraform_backend_defaults:
  type: moss
`
	ctx := newYamlCtx([]byte(data))

	gg := map[string]string{}
	for path, pos := range ctx.PathToPos {
		gg[fmt.Sprintf("%q", path)] = fmt.Sprintf("{%d, %d}", pos.Line, pos.Column)
	}
	keys := maps.Keys(gg)
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s: %s,\n", k, gg[k])
	}

	exp := map[string]Pos{
		"":                                                               {3, 1},
		"blueprint_name":                                                 {3, 17},
		"deployment_groups":                                              {27, 1},
		"deployment_groups[0]":                                           {27, 3},
		"deployment_groups[0].group":                                     {27, 10},
		"deployment_groups[0].kind":                                      {33, 9},
		"deployment_groups[0].modules":                                   {35, 3},
		"deployment_groups[0].modules[0]":                                {35, 5},
		"deployment_groups[0].modules[0].id":                             {35, 9},
		"deployment_groups[0].modules[0].kind":                           {37, 11},
		"deployment_groups[0].modules[0].outputs":                        {40, 5},
		"deployment_groups[0].modules[0].outputs[0]":                     {40, 7},
		"deployment_groups[0].modules[0].outputs[0].name":                {40, 7},
		"deployment_groups[0].modules[0].outputs[1]":                     {41, 7},
		"deployment_groups[0].modules[0].outputs[1].capppuccino":         {41, 19},
		"deployment_groups[0].modules[0].outputs[1].description":         {43, 20},
		"deployment_groups[0].modules[0].outputs[1].name":                {42, 13},
		"deployment_groups[0].modules[0].outputs[1].sensitive":           {44, 18},
		"deployment_groups[0].modules[0].settings":                       {46, 7},
		"deployment_groups[0].modules[0].settings.dijon":                 {46, 14},
		"deployment_groups[0].modules[0].settings.seaweed":               {47, 16},
		"deployment_groups[0].modules[0].settings.seaweed[0]":            {47, 17},
		"deployment_groups[0].modules[0].settings.seaweed[1]":            {47, 23},
		"deployment_groups[0].modules[0].source":                         {36, 13},
		"deployment_groups[0].modules[0].use":                            {38, 10},
		"deployment_groups[0].modules[0].use[0]":                         {38, 11},
		"deployment_groups[0].modules[0].use[1]":                         {38, 18},
		"deployment_groups[0].terraform_backend":                         {29, 5},
		"deployment_groups[0].terraform_backend.configuration":           {31, 7},
		"deployment_groups[0].terraform_backend.configuration.carrot":    {31, 15},
		"deployment_groups[0].terraform_backend.configuration.squash":    {32, 15},
		"deployment_groups[0].terraform_backend.configuration.squash[0]": {32, 16},
		"deployment_groups[0].terraform_backend.type":                    {29, 11},
		"deployment_groups[1]":                                           {49, 3},
		"deployment_groups[1].group":                                     {49, 10},
		"deployment_groups[1].modules":                                   {51, 3},
		"deployment_groups[1].modules[0]":                                {51, 5},
		"deployment_groups[1].modules[0].id":                             {51, 9},
		"deployment_groups[1].modules[1]":                                {52, 5},
		"deployment_groups[1].modules[1].id":                             {52, 9},
		"ghpc_version":                                                   {5, 15},
		"terraform_backend_defaults":                                     {55, 3},
		"terraform_backend_defaults.type":                                {55, 9},
		"validation_level":                                               {15, 19},
		"validators":                                                     {8, 1},
		"validators[0]":                                                  {8, 3},
		"validators[0].herb":                                             {11, 9},
		"validators[0].herb[0]":                                          {11, 10},
		"validators[0].herb[1]":                                          {11, 17},
		"validators[0].inputs":                                           {9, 10},
		"validators[0].spice":                                            {10, 10},
		"validators[0].validator":                                        {8, 14},
		"validators[1]":                                                  {12, 3},
		"validators[1].skip":                                             {13, 9},
		"validators[1].validator":                                        {12, 14},
		"vars":                                                           {18, 3},
		"vars.berry":                                                     {21, 5},
		"vars.berry.scarlet":                                             {22, 5},
		"vars.berry.scarlet[0]":                                          {22, 7},
		"vars.berry.scarlet[0].blood":                                    {22, 14},
		"vars.berry.scarlet[0].blood[0]":                                 {22, 15},
		"vars.berry.scarlet[0].blood[1]":                                 {22, 20},
		"vars.berry.scarlet[1]":                                          {23, 7},
		"vars.berry.scarlet[1].sangria":                                  {24, 9},
		"vars.berry.scarlet[1].sangria.merlot":                           {24, 17},
		"vars.berry.scarlet[1].sangria.merlot[0]":                        {24, 18},
		"vars.berry.scarlet[1].sangria.merlot[1]":                        {24, 28},
		"vars.blue":    {19, 9},
		"vars.blue[0]": {19, 10},
		"vars.blue[1]": {19, 17},
		"vars.red":     {18, 8},
	}

	for path, pos := range exp {
		t.Run(path, func(t *testing.T) {
			got := ctx.PathToPos[Path{path}]
			if diff := cmp.Diff(pos, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
	/*if exp == nil {
		panic("exp is nil")
	}*/
	t.Fail()

}
