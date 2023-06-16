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
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

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
			cty.StringVal("((var.kilo + 8))"),                    // HCL literal
			config.MustParseExpression("var.tina + 4").AsValue(), // HclExpression value
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

func TestHclAtttributesRW(t *testing.T) {
	want := make(map[string]cty.Value)
	// test that a string that needs escaping when written is read correctly
	want["key1"] = cty.StringVal("${value1}")

	fn, err := os.CreateTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fn.Name())

	err = WriteHclAttributes(want, fn.Name())
	if err != nil {
		t.Errorf("could not write HCL attributes file")
	}

	got, err := modulereader.ReadHclAttributes(fn.Name())
	if err != nil {
		t.Errorf("could not read HCL attributes file")
	}

	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(cty.Value{})); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}
