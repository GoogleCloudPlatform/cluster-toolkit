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
	"github.com/zclconf/go-cty/cty"
)

func TestTraversalToReference(t *testing.T) {
	type test struct {
		expr string
		want Reference
		err  bool
	}
	tests := []test{
		{"var.green", Reference{GlobalVar: true, Name: "green"}, false},
		{"var.green.sleeve", Reference{GlobalVar: true, Name: "green"}, false},
		{`var.green["sleeve"]`, Reference{GlobalVar: true, Name: "green"}, false},
		{"var.green[3]", Reference{GlobalVar: true, Name: "green"}, false},
		{"var", Reference{}, true},
		{`var["green"]`, Reference{}, true},
		{`var[3]`, Reference{}, true},
		{"local.place.here", Reference{}, true},
		{"module.pink.lime", Reference{Module: "pink", Name: "lime"}, false},
		{"module.pink.lime.red", Reference{Module: "pink", Name: "lime"}, false},
		{"module.pink.lime[3]", Reference{Module: "pink", Name: "lime"}, false},
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
			got, check := IsYamlHclLiteral(cty.StringVal(tc.input))
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.check, check); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSimpleVarToReference(t *testing.T) {
	type test struct {
		input string
		want  Reference
		err   bool
	}
	tests := []test{
		{"$(vars.green)", Reference{GlobalVar: true, Name: "green"}, false},
		{"$(var.green)", Reference{Module: "var", Name: "green"}, false},
		{"$(sleeve.green)", Reference{Module: "sleeve", Name: "green"}, false},
		{"$(box.sleeve.green)", Reference{}, true},
		{"$(vars)", Reference{}, true},
		{"$(az.buki.vedi.glagol)", Reference{}, true},
		{"gold $(var.here)", Reference{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := SimpleVarToReference(tc.input)
			if tc.err != (err != nil) {
				t.Errorf("got unexpected error: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}
