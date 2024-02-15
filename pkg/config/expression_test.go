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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty-debug/ctydebug"
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

func TestParseBpLit(t *testing.T) {
	type test struct {
		input string
		want  string
		err   bool
	}
	tests := []test{
		// Single expression, without string interpolation
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

		// String interpolation
		{`1gold was here`, `"1gold was here"`, false},
		{`2gold $(vars.here)`, `"2gold ${var.here}"`, false},
		{`3gold $(vars.here) but $(vars.gone)`, `"3gold ${var.here} but ${var.gone}"`, false},
		{`4gold
$(vars.here)`, `"4gold\n${var.here}"`, false}, // quoted strings may not be split over multiple lines

		{`5gold
was here`, `"5gold\nwas here"`, false},
		{"6gold $(vars.here", ``, true}, // missing close parenthesis

		{`#!/bin/bash
echo "Hello $(vars.project_id) from $(vars.region)"`, `"#!/bin/bash\necho \"Hello ${var.project_id} from ${var.region}\""`, false},
		{`#!/bin/bash
echo "Hello $(vars.project_id)"
`, `"#!/bin/bash\necho \"Hello ${var.project_id}\"\n"`, false},
		{"", `""`, false},
		{`$(try(vars.this) + one(vars.time))`, "try(var.this)+one(var.time)", false},

		// Escaping
		{`q $(vars.t)`, `"q ${var.t}"`, false},           // no escaping
		{`q \$(vars.t)`, `"q $(vars.t)"`, false},         // escaped `$(`
		{`q \\$(vars.t)`, `"q \\${var.t}"`, false},       // escaped `\`
		{`q \\\$(vars.t)`, `"q \\$(vars.t)"`, false},     // escaped both `\` and `$(`
		{`q \\\\$(vars.t)`, `"q \\\\${var.t}"`, false},   // escaped `\\`
		{`q \\\\\$(vars.t)`, `"q \\\\$(vars.t)"`, false}, // escaped both `\\` and `$(`

		// Translation of complex expressions BP -> Terraform
		{"$(vars.green + amber.blue)", "var.green+module.amber.blue", false},
		{"$(5 + vars.blue)", "5+var.blue", false},
		{"$(5)", "5", false},
		{`$("${vars.green}_${vars.sleeve}")`, `"${var.green}_${var.sleeve}"`, false},
		{"$(fun(vars.green))", "fun(var.green)", false},

		// Untranslatable expressions
		{"$(vars)", "", true},
		{"$(sleeve)", "", true},
		{"$(box[3])", "", true},        // can't index module
		{`$(box["green"])`, "", true},  // can't index module
		{"$(vars[3]])", "", true},      // can't index vars
		{`$(vars["green"])`, "", true}, // can't index module

	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			v, err := parseBpLit(tc.input)
			if tc.err != (err != nil) {
				t.Errorf("got unexpected error: %s", err)
			}
			if err != nil {
				return
			}
			var got string
			if v.Type() == cty.String {
				got = string(hclwrite.TokensForValue(v).Bytes())
			} else if exp, is := IsExpressionValue(v); is {
				got = string(exp.Tokenize().Bytes())
			} else {
				t.Fatalf("got value of unexpected type: %#v", v)
			}

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
			cty.NullVal(cty.String),
			cty.MapVal(map[string]cty.Value{
				"cu": cty.NumberIntVal(29),
				"ba": cty.NumberIntVal(56),
			})}),
		"pony.zebra": cty.NilVal,
		"zanzibar":   cty.NullVal(cty.DynamicPseudoType),
	})
	want := hclwrite.NewEmptyFile()
	want.Body().AppendUnstructuredTokens(hclwrite.TokensForValue(val))

	got := hclwrite.NewEmptyFile()
	got.Body().AppendUnstructuredTokens(TokensForValue(val))

	if diff := cmp.Diff(string(want.Bytes()), string(got.Bytes())); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestFlattenFunctionCallExpression(t *testing.T) {
	bp := Blueprint{Vars: NewDict(map[string]cty.Value{
		"three": cty.NumberIntVal(3),
	})}
	expr := FunctionCallExpression("flatten", cty.TupleVal([]cty.Value{
		cty.TupleVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)}),
		GlobalRef("three").AsValue(),
	}))

	want := cty.TupleVal([]cty.Value{
		cty.NumberIntVal(1),
		cty.NumberIntVal(2),
		cty.NumberIntVal(3)})

	got, err := bp.Eval(expr.AsValue())
	if err != nil {
		t.Errorf("got unexpected error: %s", err)
	}
	if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestMergeFunctionCallExpression(t *testing.T) {
	bp := Blueprint{Vars: NewDict(map[string]cty.Value{
		"fix": cty.ObjectVal(map[string]cty.Value{
			"two": cty.NumberIntVal(2),
		}),
	})}
	expr := FunctionCallExpression("merge",
		cty.ObjectVal(map[string]cty.Value{
			"one": cty.NumberIntVal(1),
			"two": cty.NumberIntVal(3),
		}),
		GlobalRef("fix").AsValue(),
	)

	want := cty.ObjectVal(map[string]cty.Value{
		"one": cty.NumberIntVal(1),
		"two": cty.NumberIntVal(2),
	})

	got, err := bp.Eval(expr.AsValue())
	if err != nil {
		t.Errorf("got unexpected error: %s", err)
	}
	if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestReplaceTokens(t *testing.T) {
	type test struct {
		body string
		old  string
		new  string
		want string
	}
	tests := []test{
		{"var.green", "var.green", "var.blue", "var.blue"},
		{"var.green + var.green", "var.green", "var.blue", "var.blue+var.blue"},
		{"vars.green + 5", "vars.green", "var.green", "var.green+5"},
		{"var.green + var.blue", "vars.gold", "var.silver", "var.green+var.blue"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("s/%s/%s/%s", tc.old, tc.new, tc.body), func(t *testing.T) {
			b, err := parseHcl(tc.body)
			if err != nil {
				t.Fatal(err)
			}
			o, err := parseHcl(tc.old)
			if err != nil {
				t.Fatal(err)
			}
			n, err := parseHcl(tc.new)
			if err != nil {
				t.Fatal(err)
			}

			got := replaceTokens(b, o, n)
			if diff := cmp.Diff(tc.want, string(got.Bytes())); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}
