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
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
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
		{Root.BlueprintName, Pos{3, 1}},
		{Root.GhpcVersion, Pos{5, 1}},
		{Root.Validators, Pos{7, 1}},
		{Root.Validators.At(0), Pos{8, 3}},
		{Root.Validators.At(0).Validator, Pos{8, 3}},
		{Root.Validators.At(0).Inputs, Pos{9, 3}},
		{Root.Validators.At(0).Inputs.Dot("spice"), Pos{10, 5}},
		{Root.Validators.At(1).Validator, Pos{11, 3}},
		{Root.Validators.At(1), Pos{11, 3}},
		{Root.Validators.At(1).Skip, Pos{12, 3}},
		{Root.ValidationLevel, Pos{14, 1}},
		{Root.Vars, Pos{16, 1}},
		{Root.Vars.Dot("red"), Pos{17, 3}},
		{Root.Groups, Pos{19, 1}},
		{Root.Groups.At(0), Pos{20, 3}},
		{Root.Groups.At(0).Name, Pos{20, 3}},

		{Root.Groups.At(0).Backend, Pos{21, 3}},
		{Root.Groups.At(0).Backend.Type, Pos{22, 5}},
		{Root.Groups.At(0).Backend.Configuration, Pos{23, 5}},
		{Root.Groups.At(0).Backend.Configuration.Dot("carrot"), Pos{24, 7}},

		{Root.Groups.At(0).Modules, Pos{25, 3}},
		{Root.Groups.At(0).Modules.At(0), Pos{26, 5}},
		{Root.Groups.At(0).Modules.At(0).ID, Pos{26, 5}},
		{Root.Groups.At(0).Modules.At(0).Source, Pos{27, 5}},
		{Root.Groups.At(0).Modules.At(0).Kind, Pos{28, 5}},
		{Root.Groups.At(0).Modules.At(0).Use, Pos{29, 5}},
		{Root.Groups.At(0).Modules.At(0).Use.At(0), Pos{29, 11}},
		{Root.Groups.At(0).Modules.At(0).Use.At(1), Pos{29, 18}},
		{Root.Groups.At(0).Modules.At(0).Outputs, Pos{30, 5}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(0), Pos{31, 7}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(0).Name, Pos{31, 7}}, // synthetic
		{Root.Groups.At(0).Modules.At(0).Outputs.At(1), Pos{32, 7}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(1).Name, Pos{32, 7}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(1).Description, Pos{33, 7}},
		{Root.Groups.At(0).Modules.At(0).Outputs.At(1).Sensitive, Pos{34, 7}},
		{Root.Groups.At(0).Modules.At(0).Settings, Pos{35, 5}},
		{Root.Groups.At(0).Modules.At(0).Settings.Dot("dijon"), Pos{36, 7}},

		{Root.Groups.At(1), Pos{38, 3}},
		{Root.Groups.At(1).Name, Pos{38, 3}},
		{Root.Groups.At(1).Modules, Pos{39, 3}},
		{Root.Groups.At(1).Modules.At(0), Pos{40, 5}},
		{Root.Groups.At(1).Modules.At(0).ID, Pos{40, 5}},
		{Root.Groups.At(1).Modules.At(1), Pos{41, 5}},
		{Root.Groups.At(1).Modules.At(1).ID, Pos{41, 5}},

		{Root.Backend, Pos{43, 1}},
		{Root.Backend.Type, Pos{44, 3}},
	}

	ctx, err := NewYamlCtx([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
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

func TestModuleKindUnmarshalYAML(t *testing.T) {
	type test struct {
		input string
		want  ModuleKind
		err   bool
	}
	tests := []test{
		{"", UnknownKind, false},
		{"terraform", TerraformKind, false},
		{"packer", PackerKind, false},

		{"unknown", ModuleKind{}, true},
		{"[]", ModuleKind{}, true},
		{"{]", ModuleKind{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			var got ModuleKind
			err := yaml.Unmarshal([]byte(tc.input), &got)
			if tc.err != (err != nil) {
				t.Fatalf("got unexpected error: %s", err)
			}

			if tc.want != got {
				t.Errorf("want:%#v:\ngot%#v", tc.want, got)
			}
		})
	}
}

func TestModuleIDsUnmarshalYAML(t *testing.T) {
	type test struct {
		input string
		want  ModuleIDs
		err   bool
	}
	tests := []test{
		{"[green, red]", ModuleIDs{"green", "red"}, false},
		{"[]", ModuleIDs{}, false},

		{"green", nil, true},
		{"44", nil, true},
		{"{}", nil, true},
		{"[[]]", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			var got ModuleIDs
			err := yaml.Unmarshal([]byte(tc.input), &got)
			if tc.err != (err != nil) {
				t.Fatalf("got unexpected error: %s", err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDictUnmarshalYAML(t *testing.T) {
	yml := `
s1: "red"
s2: pink
m1: {}	
m2:
  m2f1: green
  m2f2: [1, 0.2, -3, false]
  gv: $(vars.gold)
  mv: $(lime.bloom)
  hl: ((3 + 9))
`
	want := Dict{}
	want.
		Set("s1", cty.StringVal("red")).
		Set("s2", cty.StringVal("pink")).
		Set("m1", cty.EmptyObjectVal).
		Set("m2", cty.ObjectVal(map[string]cty.Value{
			"m2f1": cty.StringVal("green"),
			"m2f2": cty.TupleVal([]cty.Value{
				cty.NumberIntVal(1),
				cty.NumberFloatVal(0.2),
				cty.NumberIntVal(-3),
				cty.False,
			}),
			"gv": MustParseExpression("var.gold").AsValue(),
			"mv": MustParseExpression("module.lime.bloom").AsValue(),
			"hl": MustParseExpression("3 + 9").AsValue(),
		}))
	var got Dict
	if err := yaml.Unmarshal([]byte(yml), &got); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if diff := cmp.Diff(want.Items(), got.Items(), ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestDictWrongTypeUnmarshalYAML(t *testing.T) {
	yml := `
17`
	var d Dict
	err := yaml.Unmarshal([]byte(yml), &d)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if diff := cmp.Diff(err.Error(), "line 2 column 1: must be a mapping, got number"); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestDictMarshalYAML(t *testing.T) {
	d := Dict{}
	d.
		Set("s1", cty.StringVal("red")).
		Set("m1", cty.EmptyObjectVal).
		Set("m2", cty.ObjectVal(map[string]cty.Value{
			"m2f1": cty.StringVal("green"),
			"m2f2": cty.TupleVal([]cty.Value{
				cty.NumberIntVal(1),
				cty.NumberFloatVal(0.2),
				cty.NumberIntVal(-3),
				cty.False,
				MustParseExpression("7 + 4").AsValue(),
			}),
		}))
	want := map[string]interface{}{
		"s1": "red",
		"m1": map[string]interface{}{},
		"m2": map[string]interface{}{
			"m2f1": "green",
			"m2f2": []interface{}{1.0, 0.2, -3.0, false, "((7 + 4))"},
		},
	}
	got, err := d.MarshalYAML()
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestYAMLValueMarshalIntAsInt(t *testing.T) {
	d := Dict{}
	d.Set("zebra", cty.NumberIntVal(5))
	want := "zebra: 5\n"
	got, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestYAMLValueUnmarshalWithAlias(t *testing.T) {
	yml := `
pony: &passtime
- eat
- sleep
zebra: *passtime
`
	want := Dict{}
	want.
		Set("pony", cty.TupleVal([]cty.Value{cty.StringVal("eat"), cty.StringVal("sleep")})).
		Set("zebra", cty.TupleVal([]cty.Value{cty.StringVal("eat"), cty.StringVal("sleep")}))
	var got Dict
	if err := yaml.Unmarshal([]byte(yml), &got); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if diff := cmp.Diff(want.Items(), got.Items(), ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestYAMLValueUnmarshalNil(t *testing.T) {
	yml := `
a:
b: null
c: ~
d: "null"
`
	anyNull := cty.NullVal(cty.DynamicPseudoType)
	want := cty.ObjectVal(map[string]cty.Value{
		"a": anyNull,
		"b": anyNull,
		"c": anyNull,
		"d": cty.StringVal("null"),
	})

	var got YamlValue
	if err := yaml.Unmarshal([]byte(yml), &got); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if diff := cmp.Diff(want, got.Unwrap(), ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}
