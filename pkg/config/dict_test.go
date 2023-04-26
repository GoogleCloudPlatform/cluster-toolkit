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

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

func TestZeroValueValid(t *testing.T) {
	{ // Items
		d := Dict{}
		want := map[string]cty.Value{}
		got := d.Items()
		if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
			t.Errorf("diff (-want +got):\n%s", diff)
		}
	}

	{ // Has
		d := Dict{}
		if d.Has("zebra") != false {
			t.Errorf("should not contain any values")
		}
	}

	{ // Get
		d := Dict{}
		want := cty.NilVal
		got := d.Get("pony")
		if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
			t.Errorf("diff (-want +got):\n%s", diff)
		}
	}

	{ // Set
		d := Dict{}
		d.Set("lizard", cty.True) // no panic
	}
}

func TestSetAndGet(t *testing.T) {
	d := Dict{}
	want := cty.StringVal("guava")
	d.Set("hare", want)
	got := d.Get("hare")
	if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
	if !d.Has("hare") {
		t.Errorf("should have a hare")
	}
}

func TestItemsAreCopy(t *testing.T) {
	d := Dict{}
	d.Set("apple", cty.StringVal("fuji"))

	items := d.Items()
	items["apple"] = cty.StringVal("opal")

	want := cty.StringVal("fuji")
	got := d.Get("apple")
	if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestYAMLDecode(t *testing.T) {
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

func TestMarshalYAML(t *testing.T) {
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

func TestYAMLMarshalIntAsInt(t *testing.T) {
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

func TestYAMLDecodeWithAlias(t *testing.T) {
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

func TestEval(t *testing.T) {
	bp := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"zebra": cty.StringVal("stripes"),
		}),
	}
	d := NewDict(map[string]cty.Value{
		"abyss": cty.ObjectVal(map[string]cty.Value{
			"white": GlobalRef("zebra").AsExpression().AsValue(),
			"green": cty.StringVal("grass"),
		})})
	want := NewDict(map[string]cty.Value{
		"abyss": cty.ObjectVal(map[string]cty.Value{
			"white": cty.StringVal("stripes"),
			"green": cty.StringVal("grass"),
		})})
	got, err := d.Eval(bp)
	if err != nil {
		t.Fatalf("failed to eval: %v", err)
	}
	if diff := cmp.Diff(want.Items(), got.Items(), ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}
