// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modulewriter

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func TestIsHclLiteral(t *testing.T) {
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
			got, check := IsHclLiteral(cty.StringVal(tc.input))
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.check, check); diff != "" {
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
			cty.StringVal("((var.kilo + 8))")})})
	want := `
{
  tan = [var.kilo + 8]
}`[1:]

	got := hclwrite.NewEmptyFile()
	got.Body().AppendUnstructuredTokens(TokensForValue(val))

	if diff := cmp.Diff(want, string(got.Bytes())); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}
