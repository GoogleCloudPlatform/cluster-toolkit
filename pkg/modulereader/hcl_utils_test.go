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
		{"shared_standard", "projects/hcs-ext-tvc/reservations/reservation-20260928-214515", true},
		{"shared_block", "projects/hcs-ext-tvc/reservations/reservation-20261008-231448/reservationBlocks/reservation-20261008-231448-block-0001", true},
		{"shared_subblock", "projects/hcs-ext-tvc/reservations/reservation-20261008-231448/reservationBlocks/reservation-20261008-231448-block-0001/reservationSubBlocks/subblock-0001", true},
		{"invalid_subblock_without_block", "reservation-20261008-231448/reservationSubBlocks/subblock-0001", false},
		{"invalid_trailing_slash", "reservation-20261008-231448/reservationBlocks/", false},
		{"invalid_uppercase", "Reservation-20261008/reservationBlocks/block-0001", false},
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
