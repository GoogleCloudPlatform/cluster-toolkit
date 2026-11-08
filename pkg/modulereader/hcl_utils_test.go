// Copyright 2026 Google LLC
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

package modulereader

import (
	"hpc-toolkit/pkg/sourcereader"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

func (s *zeroSuite) TestNormalizeType(c *C) {
	c.Check(
		NormalizeType("object({count=number,kind=string})"),
		Equals,
		NormalizeType("object({kind=string,count=number})"))

	c.Check(NormalizeType("?invalid_type"), Equals, "?invalid_type")

	// `any` is special type, check that it works
	c.Check(NormalizeType("object({b=any,a=number})"), Equals, NormalizeType("object({a=number,b=any})"))

	c.Check(NormalizeType(" object (  {\na=any\n} ) "), Equals, NormalizeType("object({a=any})"))

	c.Check(NormalizeType(" string # comment"), Equals, NormalizeType("string"))
}

// a full-loop test of ReadWrite is implemented in modulewriter package
// focus on modes that should error
func (s *zeroSuite) TestReadHclAtttributes(c *C) {
	fn, err := os.CreateTemp("", "test-*")
	if err != nil {
		c.Fatal(err)
	}
	defer os.Remove(fn.Name())

	fn.WriteString("attribute_name = var.name")

	_, err = ReadHclAttributes(fn.Name())
	c.Assert(err, NotNil)
}

func TestReplaceTokens(t *testing.T) {
	type test struct {
		ty   string
		err  bool
		want cty.Type
	}
	tests := []test{
		{"", false, cty.DynamicPseudoType},

		{"string", false, cty.String},

		{"list", false, cty.List(cty.DynamicPseudoType)},
		{"list(string)", false, cty.List(cty.String)},
		{"list(any)", false, cty.List(cty.DynamicPseudoType)},

		{"map", false, cty.Map(cty.DynamicPseudoType)},
		{"map(string)", false, cty.Map(cty.String)},
		{"map(any)", false, cty.Map(cty.DynamicPseudoType)},

		{`object({sweet=string})`, false,
			cty.Object(map[string]cty.Type{"sweet": cty.String})},
		{`object({sweet=optional(string)})`, false,
			cty.ObjectWithOptionalAttrs(map[string]cty.Type{"sweet": cty.String}, []string{"sweet"})},
		{`object({sweet=optional(string, "caramel")})`, false,
			cty.ObjectWithOptionalAttrs(map[string]cty.Type{"sweet": cty.String}, []string{"sweet"})},

		{"for", true, cty.NilType},
	}
	for _, tc := range tests {
		t.Run(tc.ty, func(t *testing.T) {
			got, err := GetCtyType(tc.ty)
			if tc.err != (err != nil) {
				t.Errorf("got unexpected error: %s", err)
			}
			if err != nil {
				return
			}

			if !got.Equals(tc.want) {
				t.Errorf("\nwant: %#v\ngot: %#v", tc.want, got)
			}
		})
	}
}

func TestParseFileVarMetaValidationsAndDeprecations(t *testing.T) {
	src := []byte(`
variable "deprecated_null" {
  type    = string
  default = null
  validation {
    condition     = null == var.deprecated_null
    error_message = "The 'deprecated_null' variable has been deprecated. Use 'replacement' instead."
  }
}

variable "deprecated_len" {
  type    = list(string)
  default = []
  validation {
    condition     = length(var.deprecated_len) == 0
    error_message = <<-EOD
      deprecated_len is deprecated.
      Use new_list instead.
    EOD
  }
}

variable "compound_not_deprecated" {
  type    = string
  default = null
  validation {
    condition     = var.compound_not_deprecated == null || var.compound_not_deprecated != ""
    error_message = "compound_not_deprecated must not be empty when set."
  }
}
`)
	meta := make(map[string]hclVarMeta)
	parseFileVarMeta(src, "variables.tf", meta)

	if !meta["deprecated_null"].deprecated || len(meta["deprecated_null"].validations) != 0 {
		t.Fatalf("expected deprecated_null to be deprecated with 0 validations, got %+v", meta["deprecated_null"])
	}
	if !meta["deprecated_len"].deprecated || meta["deprecated_len"].deprecatedMsg != "deprecated_len is deprecated. Use new_list instead." {
		t.Fatalf("expected deprecated_len heredoc message to normalize cleanly, got %+v", meta["deprecated_len"])
	}
	if meta["compound_not_deprecated"].deprecated || len(meta["compound_not_deprecated"].validations) != 1 {
		t.Fatalf("expected compound_not_deprecated to have 1 validation and not be deprecated, got %+v", meta["compound_not_deprecated"])
	}
}

func TestApplyVarMetaClonesInputs(t *testing.T) {
	orig := ModuleInfo{
		Inputs: []VarInfo{{Name: "legacy_flag", Description: "DEPRECATED: use new_flag instead."}},
	}
	enriched := ApplyVarMeta("nonexistent-source", orig)
	if orig.Inputs[0].Deprecated {
		t.Fatalf("expected ApplyVarMeta not to mutate original Inputs slice")
	}
	if !enriched.Inputs[0].Deprecated || enriched.Inputs[0].DeprecatedMsg != "DEPRECATED: use new_flag instead." {
		t.Fatalf("expected enriched input to be marked deprecated, got %+v", enriched.Inputs[0])
	}
}

func TestApplyVarMetaReadsLocalSources(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"variables.tf": `
variable "legacy" {
  type    = string
  default = null
  validation {
    condition     = var.legacy == null
    error_message = "legacy is deprecated; use modern."
  }
}
variable "size" {
  type    = number
  default = 1
  validation {
    condition     = var.size > 0
    error_message = "size must be positive (got ${var.size})."
  }
}
`,
		"variables.pkr.hcl": `
variable "old_scopes" {
  type    = list(string)
  default = null
  validation {
    condition     = var.old_scopes == null
    error_message = "old_scopes is deprecated; use scopes."
  }
}
`,
		"broken.tf": "variable {",
		"README.md": "not a variable file",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mi := ApplyVarMeta(dir, ModuleInfo{Inputs: []VarInfo{{Name: "legacy"}, {Name: "size"}, {Name: "old_scopes"}}})
	if !mi.Inputs[0].Deprecated || mi.Inputs[0].DeprecatedMsg != "legacy is deprecated; use modern." {
		t.Fatalf("expected legacy deprecated via validation, got %+v", mi.Inputs[0])
	}
	if mi.Inputs[1].Deprecated || len(mi.Inputs[1].Validations) != 1 || !strings.Contains(mi.Inputs[1].Validations[0], "${var.size}") {
		t.Fatalf("expected size to keep its raw interpolated validation message, got %+v", mi.Inputs[1])
	}
	if !mi.Inputs[2].Deprecated {
		t.Fatalf("expected old_scopes from .pkr.hcl to be deprecated, got %+v", mi.Inputs[2])
	}
}

func TestApplyVarMetaReadsEmbeddedSources(t *testing.T) {
	oldFS := sourcereader.ModuleFS
	defer func() { sourcereader.ModuleFS = oldFS }()
	sourcereader.ModuleFS = testModuleFS

	for _, src := range []string{"modules/test_role/test_module", "modules/imaginarium/zebra"} {
		emb := ApplyVarMeta(src, ModuleInfo{Inputs: []VarInfo{{Name: "test_variable", Description: "This is just a test"}}})
		if emb.Inputs[0].Deprecated || len(emb.Inputs[0].Validations) != 0 {
			t.Fatalf("%s: expected no metadata for test_variable, got %+v", src, emb.Inputs[0])
		}
	}
	if got := extractHCLVarMeta("modules/does-not-exist"); got != nil {
		t.Fatalf("expected nil metadata for a missing embedded module, got %+v", got)
	}
}
