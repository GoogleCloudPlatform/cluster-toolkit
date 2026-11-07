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
    condition     = var.deprecated_null == null
    error_message = "The 'deprecated_null' variable has been deprecated. Use 'replacement' instead."
  }
}

variable "deprecated_desc" {
  description = "DEPRECATED: use new_var instead."
  type        = bool
  default     = false
}

variable "storage_type" {
  description = "Storage type to configure."
  type        = string
  validation {
    condition     = contains(["GCSFuse", "Lustre"], var.storage_type)
    error_message = "Allowed values are GCSFuse or Lustre."
  }
}
`)
	meta := make(map[string]hclVarMeta)
	parseFileVarMeta(src, "variables.tf", meta)

	if !meta["deprecated_null"].deprecated || meta["deprecated_null"].deprecatedMsg == "" {
		t.Fatalf("expected deprecated_null to be marked deprecated, got %+v", meta["deprecated_null"])
	}
	if len(meta["deprecated_null"].validations) != 0 {
		t.Fatalf("expected deprecated_null validations to be empty, got %+v", meta["deprecated_null"].validations)
	}
	if meta["storage_type"].deprecated || len(meta["storage_type"].validations) != 1 {
		t.Fatalf("expected storage_type to have 1 validation and not be deprecated, got %+v", meta["storage_type"])
	}

	descMeta := applyDescriptionDeprecation("DEPRECATED: use new_var instead.", hclVarMeta{})
	if !descMeta.deprecated || descMeta.deprecatedMsg != "DEPRECATED: use new_var instead." {
		t.Fatalf("expected deprecated_desc to be marked deprecated from description, got %+v", descMeta)
	}
}
