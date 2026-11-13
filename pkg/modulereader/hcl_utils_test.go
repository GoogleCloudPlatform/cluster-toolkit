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
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
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
    error_message = "size must be positive\n(got ${var.size})."
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
	if mi.Inputs[1].Deprecated || len(mi.Inputs[1].Validations) != 1 || mi.Inputs[1].Validations[0] != "size must be positive (got ${var.size})." {
		t.Fatalf("expected size to unescape template and preserve ${var.size}, got %+v", mi.Inputs[1])
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

func extractVariableValidationCondition(t *testing.T, varsFile, varName string) hclsyntax.Expression {
	t.Helper()
	f, diags := hclparse.NewParser().ParseHCLFile(varsFile)
	if diags.HasErrors() {
		t.Fatalf("failed to parse %s: %v", varsFile, diags)
	}
	body, ok := f.Body.(*hclsyntax.Body)
	if !ok {
		t.Fatalf("expected *hclsyntax.Body, got %T", f.Body)
	}
	for _, blk := range body.Blocks {
		if blk.Type != "variable" || len(blk.Labels) != 1 || blk.Labels[0] != varName {
			continue
		}
		for _, sub := range blk.Body.Blocks {
			if sub.Type == "validation" {
				attr, ok := sub.Body.Attributes["condition"]
				if !ok {
					t.Fatalf("validation block in variable %s does not have a 'condition' attribute", varName)
				}
				return attr.Expr
			}
		}
	}

	t.Fatalf("could not find validation.condition for variable %s in %s", varName, varsFile)
	return nil
}

func TestVMInstanceReservationNameValidationHCL(t *testing.T) {
	varsFile := filepath.Join("..", "..", "modules", "compute", "vm-instance", "variables.tf")
	condExpr := extractVariableValidationCondition(t, varsFile, "reservation_name")

	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{"empty_string", "", true},
		{"local_standard_reservation", "reservation-20260928-214515", true},
		{"local_dense_block_reservation", "reservation-20261008-231448/reservationBlocks/reservation-20261008-231448-block-0001", true},
		{"local_subblock", "reservation-20261008-231448/reservationBlocks/reservation-20261008-231448-block-0001/reservationSubBlocks/reservation-20261008-231448-subblock-0001", true},
		{"shared_standard", "projects/consumer-proj/reservations/reservation-20260928-214515", true},
		{"shared_block", "projects/consumer-proj/reservations/reservation-20261008-231448/reservationBlocks/reservation-20261008-231448-block-0001", true},
		{"shared_subblock", "projects/consumer-proj/reservations/reservation-20261008-231448/reservationBlocks/reservation-20261008-231448-block-0001/reservationSubBlocks/subblock-0001", true},
		{"invalid_subblock_without_block", "reservation-20261008-231448/reservationSubBlocks/subblock-0001", false},
		{"invalid_empty_block_with_subblock", "reservation-20261008-231448/reservationBlocks//reservationSubBlocks/subblock-0001", false},
		{"invalid_trailing_slash", "reservation-20261008-231448/reservationBlocks/", false},
		{"invalid_subblock_trailing_slash", "reservation-20261008-231448/reservationBlocks/block-0001/reservationSubBlocks/", false},
		{"invalid_uppercase", "Reservation-20261008/reservationBlocks/block-0001", false},
		{"invalid_uppercase_block", "reservation-20261008/reservationBlocks/Block-0001", false},
		{"invalid_extra_segment", "reservation-20261008-231448/reservationBlocks/block-0001/reservationSubBlocks/sub-0001/extra", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evalCtx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"var": cty.ObjectVal(map[string]cty.Value{
						"reservation_name": cty.StringVal(tc.value),
					}),
				},
				Functions: map[string]function.Function{
					"length":   stdlib.LengthFunc,
					"regexall": stdlib.RegexAllFunc,
				},
			}
			val, diags := condExpr.Value(evalCtx)
			if diags.HasErrors() {
				t.Fatalf("HCL evaluation error for %q: %v", tc.value, diags)
			}
			if val.True() != tc.valid {
				t.Errorf("reservation_name=%q: got valid=%v, want %v", tc.value, val.True(), tc.valid)
			}
		})
	}
}
