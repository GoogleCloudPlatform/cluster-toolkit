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
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func TestTraversalToReference(t *testing.T) {
	type test struct {
		expr string
		want Reference
		err  bool
	}
	tests := []test{
		{"var.green", GlobalRef("green"), false},
		{"var.green.sleeve", GlobalRef("green"), false},
		{`var.green["sleeve"]`, GlobalRef("green"), false},
		{"var.green[3]", GlobalRef("green"), false},
		{"var", Reference{}, true},
		{`var["green"]`, Reference{}, true},
		{`var[3]`, Reference{}, true},
		{"local.place.here", Reference{}, true},
		{"module.pink.lime", ModuleRef("pink", "lime"), false},
		{"module.pink.lime.red", ModuleRef("pink", "lime"), false},
		{"module.pink.lime[3]", ModuleRef("pink", "lime"), false},
		{"module.pink", Reference{}, true},
		{`module.pink["lime"]`, Reference{}, true},
		{"module.pink[3]", Reference{}, true},
		{`module["lime"]`, Reference{}, true},
		{"module[3]", Reference{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			e, diag := hclsyntax.ParseExpression([]byte(tc.expr), "", hcl.Pos{})
			if diag.HasErrors() {
				t.Fatal(diag)
			}
			te, ok := e.(*hclsyntax.ScopeTraversalExpr)
			if !ok {
				t.Fatalf("expected traversal expression, got %#v", e)
			}
			got, err := TraversalToReference(te.AsTraversal())
			if tc.err != (err != nil) {
				t.Fatalf("got unexpected error: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}

		})
	}
}

func TestIsYamlHclLiteral(t *testing.T) {
	type test struct {
		input string
		want  string
		check bool
	}
	tests := []test{
		{"((var.green))", "var.green", true},
		{"((${var.green}))", "${var.green}", true},
		{"(( 7 + a }))", " 7 + a }", true},
		{"(var.green)", "", false},
		{"((var.green)", "", false},
		{"$(var.green)", "", false},
		{"${var.green}", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, check := IsYamlExpressionLiteral(cty.StringVal(tc.input))
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.check, check); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSimpleVarToExpression(t *testing.T) {
	type test struct {
		input string
		want  string
		err   bool
	}
	tests := []test{
		{"$(vars.green)", "var.green", false},
		{"$(vars.green[3])", "var.green[3]", false},
		{"$(vars.green.sleeve)", "var.green.sleeve", false},
		{`$(vars.green["sleeve"])`, `var.green["sleeve"]`, false},
		{"$(vars.green.sleeve[3])", "var.green.sleeve[3]", false},

		{"$(var.green)", "module.var.green", false},
		{"$(box.green)", "module.box.green", false},
		{"$(box.green.sleeve)", "module.box.green.sleeve", false},
		{"$(box.green[3])", "module.box.green[3]", false},
		{"$(box.green.sleeve[3])", "module.box.green.sleeve[3]", false},
		{`$(box.green["sleeve"])`, `module.box.green["sleeve"]`, false},

		{"$(vars)", "", true},
		{"$(sleeve)", "", true},
		{"gold $(var.here)", "", true},
		{"$(box[3])", "", true},        // can't index module
		{`$(box["green"])`, "", true},  // can't index module
		{"$(vars[3]])", "", true},      // can't index vars
		{`$(vars["green"])`, "", true}, // can't index module
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			exp, err := SimpleVarToExpression(tc.input)
			if tc.err != (err != nil) {
				t.Errorf("got unexpected error: %s", err)
			}
			if err != nil {
				return
			}
			got := string(exp.Tokenize().Bytes())
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTokensForValueNoLiteral(t *testing.T) {
	val := cty.ObjectVal(map[string]cty.Value{
		"tan": cty.TupleVal([]cty.Value{
			cty.StringVal("biege"),
			cty.MapVal(map[string]cty.Value{
				"cu": cty.NumberIntVal(29),
				"ba": cty.NumberIntVal(56),
			})}),
		"pony.zebra": cty.NilVal,
	})
	want := hclwrite.NewEmptyFile()
	want.Body().AppendUnstructuredTokens(hclwrite.TokensForValue(val))

	got := hclwrite.NewEmptyFile()
	got.Body().AppendUnstructuredTokens(TokensForValue(val))

	if diff := cmp.Diff(string(want.Bytes()), string(got.Bytes())); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestTokensForValueWithLiteral(t *testing.T) {
	val := cty.ObjectVal(map[string]cty.Value{
		"tan": cty.TupleVal([]cty.Value{
			cty.StringVal("((var.kilo + 8))"),             // HCL literal
			MustParseExpression("var.tina + 4").AsValue(), // HclExpression value
		})})
	want := `
{
  tan = [var.kilo + 8, var.tina + 4]
}`[1:]

	gotF := hclwrite.NewEmptyFile()
	gotF.Body().AppendUnstructuredTokens(TokensForValue(val))
	got := hclwrite.Format(gotF.Bytes()) // format to normalize whitespace

	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}
