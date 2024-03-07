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

func TestShowcaseDangersOfHclWrite(t *testing.T) {
	// NOTE: if this test fails, it's not necessarily a bug in the code, but hclwrite got fixed.
	// Feel free to remove it in that case.
	toks := config.MustParseExpression(`var.green`).Tokenize()
	good, bad, ugly := "var.green", " var.green ", "var.green "

	// original
	if diff := cmp.Diff(good, string(toks.Bytes())); diff != "" {
		t.Errorf("diff (-good +got):\n%s", diff)
	}

	// affected by HclFile SetAttributeRaw & Format
	f := hclwrite.NewEmptyFile()
	f.Body().SetAttributeRaw("zz", toks) // no side effects yet

	if diff := cmp.Diff(good, string(toks.Bytes())); diff != "" {
		t.Errorf("diff (-still_good +got):\n%s", diff)
	}

	hclwrite.Format(f.Bytes()) // side effect happens here, for some reason

	if diff := cmp.Diff(bad, string(toks.Bytes())); diff != "" {
		t.Errorf("diff (-bad +got):\n%s", diff)
	}

	// leads to ugly post-formatting
	finFormat := hclwrite.Format(toks.Bytes())
	if diff := cmp.Diff(ugly, string(finFormat)); diff != "" {
		t.Errorf("diff (-ugly +got):\n%s", diff)
	}
}
