// Copyright 2026 Google LLC
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

package modulereader

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// conditionRules parses a validation condition for var.x and returns the
// derived rule, if any, ready to pass to buildInputConstraints.
func conditionRules(t *testing.T, cond string) []ruleConstraints {
	t.Helper()
	expr, diags := hclsyntax.ParseExpression([]byte(cond), "variables.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("failed to parse %q: %v", cond, diags)
	}
	rc, ok := conditionAttrConstraints("x", &hclsyntax.Attribute{Name: "condition", Expr: expr})
	if !ok {
		return nil
	}
	return []ruleConstraints{rc}
}

func constraintsJSON(t *testing.T, c *InputConstraints) string {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestConditionConstraints(t *testing.T) {
	tests := []struct {
		name string
		cond string
		want string
	}{
		// Shapes that are understood.
		{"contains", `contains(["a", "b"], var.x)`,
			`{"allowed_values":["a","b"],"sources":["terraform"]}`},
		{"contains bools", `contains([true, false], var.x)`,
			`{"allowed_values":[true,false],"sources":["terraform"]}`},
		{"null or contains", `var.x == null || contains(["MIGRATE", "TERMINATE"], var.x)`,
			`{"allowed_values":["MIGRATE","TERMINATE"],"allow_null":true,"sources":["terraform"]}`},
		{"contains or null reversed", `contains(["a"], var.x) || null == var.x`,
			`{"allowed_values":["a"],"allow_null":true,"sources":["terraform"]}`},
		{"can regex or null reversed", `can(regex("^a", var.x)) || var.x == null`,
			`{"pattern":"^a","allow_null":true,"sources":["terraform"]}`},
		{"bound or null reversed drops allow_null", `var.x > 0 || var.x == null`,
			`{"min":0,"min_exclusive":true,"sources":["terraform"]}`},
		{"folded contains or null reversed drops allow_null", `contains(["a"], lower(var.x)) || var.x == null`,
			`{"allowed_values":["a"],"case_insensitive":true,"sources":["terraform"]}`},
		{"regexall or null reversed drops allow_null", `length(regexall("^a", var.x)) > 0 || var.x == null`,
			`{"pattern":"^a","sources":["terraform"]}`},
		{"negative zero normalized", `var.x >= -0`,
			`{"min":0,"sources":["terraform"]}`},
		{"ternary null true", `var.x == null ? true : contains(["MIGRATE", "TERMINATE"], var.x)`,
			`{"allowed_values":["MIGRATE","TERMINATE"],"allow_null":true,"sources":["terraform"]}`},
		{"ternary null false", `var.x == null ? false : contains(["a"], var.x)`,
			`{"allowed_values":["a"],"sources":["terraform"]}`},
		{"ternary not null", `var.x != null ? contains(["a"], var.x) : true`,
			`{"allowed_values":["a"],"allow_null":true,"sources":["terraform"]}`},
		{"numbers with parens", `(var.x == null) || (contains([1, 2], var.x))`,
			`{"allowed_values":[1,2],"allow_null":true,"sources":["terraform"]}`},
		{"can regex", `can(regex("^[a-z]+$", var.x))`,
			`{"pattern":"^[a-z]+$","sources":["terraform"]}`},
		{"regexall greater than zero", `length(regexall("^projects/.+$", var.x)) > 0`,
			`{"pattern":"^projects/.+$","sources":["terraform"]}`},
		{"regexall not zero reversed", `0 != length(regexall("^a", var.x))`,
			`{"pattern":"^a","sources":["terraform"]}`},
		{"exclusive range with null", `var.x == null ? true : var.x > 0 && var.x < 65536`,
			`{"min":0,"min_exclusive":true,"max":65536,"max_exclusive":true,"allow_null":true,"sources":["terraform"]}`},
		{"negative min", `var.x >= -1`,
			`{"min":-1,"sources":["terraform"]}`},
		{"number on the left less", `0 < var.x`,
			`{"min":0,"min_exclusive":true,"sources":["terraform"]}`},
		{"number on the left lte", `1 <= var.x`,
			`{"min":1,"sources":["terraform"]}`},
		{"number on the left greater", `10 > var.x`,
			`{"max":10,"max_exclusive":true,"sources":["terraform"]}`},
		{"number on the left gte", `10 >= var.x`,
			`{"max":10,"sources":["terraform"]}`},
		{"range with unknown part", `var.x >= 1 && var.x <= 10 && floor(var.x) == var.x`,
			`{"min":1,"max":10,"sources":["terraform"]}`},
		{"and keeps right understood part", `length(var.x) > 0 && can(regex("^[a-z]+$", var.x))`,
			`{"pattern":"^[a-z]+$","sources":["terraform"]}`},
		{"null or bound", `var.x == null || var.x > 0`,
			`{"min":0,"min_exclusive":true,"allow_null":true,"sources":["terraform"]}`},
		{"tighter min wins", `var.x > 5 && var.x > 3`,
			`{"min":5,"min_exclusive":true,"sources":["terraform"]}`},
		{"equal min becomes exclusive", `var.x >= 5 && var.x > 5`,
			`{"min":5,"min_exclusive":true,"sources":["terraform"]}`},
		{"lower case-insensitive", `contains(["tigervnc", "turbovnc"], lower(var.x))`,
			`{"allowed_values":["tigervnc","turbovnc"],"case_insensitive":true,"sources":["terraform"]}`},
		{"upper case-insensitive", `contains(["STANDARD", "PREMIUM"], upper(var.x))`,
			`{"allowed_values":["STANDARD","PREMIUM"],"case_insensitive":true,"sources":["terraform"]}`},
		{"and keeps understood part", `contains(["a", "b"], var.x) && length(var.x) > 0`,
			`{"allowed_values":["a","b"],"sources":["terraform"]}`},
		{"not null and regex", `var.x != null && can(regex("^[a-z]+$", var.x))`,
			`{"pattern":"^[a-z]+$","sources":["terraform"]}`},
		{"null claim dropped next to unknown part", `(var.x == null || contains(["a"], var.x)) && length(coalesce(var.x, "")) < 10`,
			`{"allowed_values":["a"],"sources":["terraform"]}`},

		// Shapes that must not produce limits.
		{"mixed case with lower", `contains(["TigerVNC"], lower(var.x))`, `null`},
		{"transformed value", `contains(["tigervnc"], lower(trimspace(var.x)))`, `null`},
		{"negation", `!contains(["a"], var.x)`, `null`},
		{"length is not a max", `length(var.x) <= 5`, `null`},
		{"cross variable bound", `var.x >= var.y`, `null`},
		{"or without null guard", `contains(["a"], var.x) || var.x == "custom"`, `null`},
		{"other variable", `contains(["a"], var.other)`, `null`},
		{"interpolated pattern", `can(regex("^${local.prefix}-[a-z]+$", var.x))`, `null`},
		{"non-RE2 pattern", `can(regex("^(?=a)b$", var.x))`, `null`},
		{"regexall must not match", `length(regexall("bad", var.x)) == 0`, `null`},
		{"empty list", `contains([], var.x)`, `null`},
		{"null in list", `contains(["a", null], var.x)`, `null`},
		{"equality is not a bound", `var.x == 5`, `null`},
		{"loop over elements", `alltrue([for v in var.x : contains(["a"], v)])`, `null`},
		{"attribute of var", `contains(["V2", "V3"], var.x.version)`, `null`},
		{"contradictory bounds", `var.x >= 10 && var.x <= 5`, `null`},
		{"contradictory exclusive equal bounds", `var.x > 5 && var.x <= 5`, `null`},
		{"overflow bound", `var.x <= 1e400`, `null`},
		{"overflow list item", `contains([1, 1e400], var.x)`, `null`},
		{"expand final in length", `length(regexall("^a", var.x)...) > 0`, `null`},
		{"expand final in can", `can(regex("^a", var.x)...)`, `null`},
		{"expand final in lower", `contains(["a"], lower(var.x...))`, `null`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := constraintsJSON(t, buildInputConstraints(cty.DynamicPseudoType, conditionRules(t, tc.cond)))
			if got != tc.want {
				t.Errorf("condition %q\nwant: %s\ngot:  %s", tc.cond, tc.want, got)
			}
		})
	}
}

func TestBuildInputConstraintsTypeGate(t *testing.T) {
	rules := conditionRules(t, `contains(["a"], var.x)`)
	for _, typ := range []cty.Type{cty.String, cty.Number, cty.Bool, cty.DynamicPseudoType} {
		if buildInputConstraints(typ, rules) == nil {
			t.Errorf("expected limits for type %s", typ.FriendlyName())
		}
	}
	for _, typ := range []cty.Type{cty.List(cty.String), cty.Map(cty.String), cty.Object(map[string]cty.Type{"a": cty.String})} {
		if got := buildInputConstraints(typ, rules); got != nil {
			t.Errorf("expected no limits for type %s, got %+v", typ.FriendlyName(), got)
		}
	}
}

func TestMetadataVarInfo(t *testing.T) {
	list := func(items ...interface{}) []interface{} { return items }
	inputTypes := map[string]cty.Type{
		"tier":                cty.String,
		"backend":             cty.String,
		"mode":                cty.String,
		"str_from_nums":       cty.String,
		"num_from_strs":       cty.Number,
		"flag":                cty.Bool,
		"untyped_enum":        cty.DynamicPseudoType,
		"bad_enum":            cty.Number,
		"size_gb":             cty.Number,
		"frac_range":          cty.Number,
		"bad_range":           cty.Number,
		"inf_range":           cty.Number,
		"untyped_range":       cty.DynamicPseudoType,
		"zones":               cty.DynamicPseudoType,
		"deployment_name":     cty.String,
		"name":                cty.String,
		"bucket_uri":          cty.String,
		"num_regex":           cty.Number,
		"placement_policy":    cty.DynamicPseudoType,
		"image":               cty.String,
		"enable_dynamic_tier": cty.Bool,
		"throughput":          cty.Number,
		"dws_flex":            cty.DynamicPseudoType,
		"enable_placement":    cty.Bool,
		"machine_type":        cty.String,
	}
	rules := []ValidationRule{
		{Validator: "allowed_enum", ErrorMessage: "tier must be BASIC or ZONAL.",
			Inputs: map[string]interface{}{"vars": list("tier"), "allowed": list("BASIC", "ZONAL"), "allow_null": true}},
		{Validator: "allowed_enum", ErrorMessage: "backend must be tigervnc.",
			Inputs: map[string]interface{}{"vars": list("backend"), "allowed": list("tigervnc"), "case_sensitive": false}},
		{Validator: "allowed_enum", ErrorMessage: "mode must be a.",
			Inputs: map[string]interface{}{"vars": list("mode"), "allowed": list("a"), "case_sensitive": "no"}},
		{Validator: "allowed_enum", ErrorMessage: "str_from_nums must be 1 or 2.",
			Inputs: map[string]interface{}{"vars": list("str_from_nums"), "allowed": list(1, 2)}},
		{Validator: "allowed_enum", ErrorMessage: "num_from_strs must be 80 or 443.",
			Inputs: map[string]interface{}{"vars": list("num_from_strs"), "allowed": list("80", "443")}},
		{Validator: "allowed_enum", ErrorMessage: "flag ignored by AllowedEnumValidator.",
			Inputs: map[string]interface{}{"vars": list("flag"), "allowed": list(true, false)}},
		{Validator: "allowed_enum", ErrorMessage: "untyped_enum skipped because any accepts [].",
			Inputs: map[string]interface{}{"vars": list("untyped_enum"), "allowed": list("a")}},
		{Validator: "allowed_enum", ErrorMessage: "bad float skipped.",
			Inputs: map[string]interface{}{"vars": list("bad_enum"), "allowed": list(math.NaN())}},
		{Validator: "range", ErrorMessage: "size_gb must be between 1024 and 65536.",
			Inputs: map[string]interface{}{"vars": list("size_gb"), "min": 1024, "max": float64(65536)}},
		{Validator: "range", ErrorMessage: "frac_range skipped.",
			Inputs: map[string]interface{}{"vars": list("frac_range"), "min": 1.5}},
		{Validator: "range", ErrorMessage: "bad range skipped.",
			Inputs: map[string]interface{}{"vars": list("bad_range"), "min": 10, "max": 5}},
		{Validator: "range", ErrorMessage: "inf range skipped.",
			Inputs: map[string]interface{}{"vars": list("inf_range"), "max": math.Inf(1)}},
		{Validator: "range", ErrorMessage: "untyped_range skipped.",
			Inputs: map[string]interface{}{"vars": list("untyped_range"), "min": 1}},
		{Validator: "range", ErrorMessage: "need at least one zone.",
			Inputs: map[string]interface{}{"vars": list("zones"), "min": 1, "length_check": true}},
		{Validator: "regex", ErrorMessage: "combined id is invalid.",
			Inputs: map[string]interface{}{"vars": list("deployment_name", "name"), "concat": true, "separator": "-", "pattern": "^[a-z][-a-z0-9]{4,28}[a-z0-9]$"}},
		{Validator: "regex", ErrorMessage: "bucket_uri must start with gs://.",
			Inputs: map[string]interface{}{"vars": list("bucket_uri"), "pattern": "^(gs://.*)?$"}},
		{Validator: "regex", ErrorMessage: "num_regex ignored on number.",
			Inputs: map[string]interface{}{"vars": list("num_regex"), "pattern": "^[0-9]+$"}},
		{Validator: "allowed_enum", ErrorMessage: "placement_policy.type must be COMPACT.",
			Inputs: map[string]interface{}{"vars": list("placement_policy.type"), "allowed": list("COMPACT")}},
		{Validator: "range", ErrorMessage: "max_surge must be at least 0.",
			Inputs: map[string]interface{}{"vars": list("local.upgrade_settings.max_surge"), "min": 0}},
		{Validator: "regex", Level: "warning", ErrorMessage: "image should be a full path.",
			Inputs: map[string]interface{}{"vars": list("image"), "pattern": "^projects/"}},
		{Validator: "conditional", ErrorMessage: "throughput must not be set with dynamic tier.",
			Inputs: map[string]interface{}{"trigger": "enable_dynamic_tier", "trigger_value": true, "dependent": "throughput", "dependent_value": nil}},
		{Validator: "conditional_regex", ErrorMessage: "compact placement requires A3/A4 when DWS Flex is enabled.",
			Inputs: map[string]interface{}{
				"triggers":  map[string]interface{}{"dws_flex.enabled": true, "enable_placement": true},
				"dependent": "machine_type",
				"pattern":   "^a[34]",
			}},
		// Malformed mixed-type vars list must be skipped cleanly.
		{Validator: "allowed_enum", ErrorMessage: "malformed vars.",
			Inputs: map[string]interface{}{"vars": list("bad_vars", 42), "allowed": list("a")}},
	}
	info := metadataVarInfo(rules, inputTypes)

	tests := []struct {
		name string
		want string
	}{
		{"tier", `{"allowed_values":["BASIC","ZONAL"],"allow_null":true,"sources":["metadata"]}`},
		{"backend", `{"allowed_values":["tigervnc"],"case_insensitive":true,"sources":["metadata"]}`},
		{"mode", `null`}, // unreadable flag: rule skipped
		{"str_from_nums", `{"allowed_values":["1","2"],"sources":["metadata"]}`},
		{"num_from_strs", `{"allowed_values":[80,443],"sources":["metadata"]}`},
		{"flag", `null`},         // AllowedEnumValidator skips cty.Bool
		{"untyped_enum", `null`}, // any accepts []
		{"bad_enum", `null`},     // NaN element skipped
		{"size_gb", `{"min":1024,"max":65536,"sources":["metadata"]}`},
		{"frac_range", `null`},    // fractional float truncated by parseIntInput
		{"bad_range", `null`},     // min > max skipped
		{"inf_range", `null`},     // +Inf skipped
		{"untyped_range", `null`}, // range requires cty.Number
		{"zones", `null`},         // length_check limits list length
		{"deployment_name", `null`},
		{"name", `null`},
		{"bucket_uri", `{"pattern":"^(gs://.*)?$","sources":["metadata"]}`},
		{"num_regex", `null`},        // RegexValidator skips non-string
		{"placement_policy", `null`}, // nested attribute
		{"image", `null`},            // warning-level rule
		{"enable_dynamic_tier", `null`},
		{"throughput", `null`},
		{"dws_flex", `null`},
		{"enable_placement", `null`},
		{"machine_type", `null`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := constraintsJSON(t, buildInputConstraints(inputTypes[tc.name], info[tc.name].rules))
			if got != tc.want {
				t.Errorf("want: %s\ngot:  %s", tc.want, got)
			}
			if len(info[tc.name].messages) != 1 {
				t.Errorf("expected exactly 1 message for %s, got %q", tc.name, info[tc.name].messages)
			}
		})
	}
	if _, ok := info["local"]; ok {
		t.Errorf("local.* targets must not be treated as inputs")
	}
	if _, ok := info["bad_vars"]; ok {
		t.Errorf("malformed mixed-type vars list must be rejected")
	}
}

func TestBuildInputConstraintsMergesSources(t *testing.T) {
	list := func(items ...interface{}) []interface{} { return items }
	meta := func(typ cty.Type, r ValidationRule) []ruleConstraints {
		r.Inputs["vars"] = list("x")
		return metadataVarInfo([]ValidationRule{r}, map[string]cty.Type{"x": typ})["x"].rules
	}

	tests := []struct {
		name string
		typ  cty.Type
		tf   string
		meta ValidationRule
		want string
	}{
		{"same list from both", cty.Number,
			`var.x == null || contains([125, 250, 500, 1000], var.x)`,
			ValidationRule{Validator: "allowed_enum", Inputs: map[string]interface{}{"allowed": list(125, 250, 500, 1000), "allow_null": true}},
			`{"allowed_values":[125,250,500,1000],"allow_null":true,"sources":["metadata","terraform"]}`},
		{"numeric string enum merges with tf numbers", cty.Number,
			`contains([80, 443], var.x)`,
			ValidationRule{Validator: "allowed_enum", Inputs: map[string]interface{}{"allowed": list("80", "443")}},
			`{"allowed_values":[80,443],"sources":["metadata","terraform"]}`},
		{"negative zero matches zero", cty.Number,
			`contains([-0, 1], var.x)`,
			ValidationRule{Validator: "allowed_enum", Inputs: map[string]interface{}{"allowed": list(0, 1)}},
			`{"allowed_values":[0,1],"sources":["metadata","terraform"]}`},
		{"intersection", cty.String,
			`contains(["a", "b", "c"], var.x)`,
			ValidationRule{Validator: "allowed_enum", Inputs: map[string]interface{}{"allowed": list("b", "c", "d")}},
			`{"allowed_values":["b","c"],"sources":["metadata","terraform"]}`},
		{"case-sensitive intersects case-insensitive", cty.String,
			`contains(["Foo", "Bar"], var.x)`,
			ValidationRule{Validator: "allowed_enum", Inputs: map[string]interface{}{"allowed": list("foo"), "case_sensitive": false}},
			`{"allowed_values":["Foo"],"sources":["metadata","terraform"]}`},
		{"case-sensitive disjoint from case-insensitive", cty.String,
			`contains(["Foo", "Bar"], var.x)`,
			ValidationRule{Validator: "allowed_enum", Inputs: map[string]interface{}{"allowed": list("baz"), "case_sensitive": false}},
			`null`},
		{"metadata display casing kept on CI merge", cty.String,
			`contains(["readwriteonce", "readonlymany"], lower(var.x))`,
			ValidationRule{Validator: "allowed_enum", Inputs: map[string]interface{}{"allowed": list("ReadWriteOnce", "ReadOnlyMany"), "case_sensitive": false}},
			`{"allowed_values":["ReadWriteOnce","ReadOnlyMany"],"case_insensitive":true,"sources":["metadata","terraform"]}`},
		{"larger min wins", cty.Number,
			`var.x >= 1`,
			ValidationRule{Validator: "range", Inputs: map[string]interface{}{"min": 1024}},
			`{"min":1024,"sources":["metadata","terraform"]}`},
		{"exclusive max wins on tie", cty.Number,
			`var.x < 10`,
			ValidationRule{Validator: "range", Inputs: map[string]interface{}{"max": 10}},
			`{"max":10,"max_exclusive":true,"sources":["metadata","terraform"]}`},
		{"contradictory bounds across sources", cty.Number,
			`var.x >= 100`,
			ValidationRule{Validator: "range", Inputs: map[string]interface{}{"max": 50}},
			`null`},
		{"contradiction gives nothing", cty.String,
			`contains(["a"], var.x)`,
			ValidationRule{Validator: "allowed_enum", Inputs: map[string]interface{}{"allowed": list("b")}},
			`null`},
		{"null needs both enum sources", cty.String,
			`var.x == null || contains(["a"], var.x)`,
			ValidationRule{Validator: "allowed_enum", Inputs: map[string]interface{}{"allowed": list("a")}},
			`{"allowed_values":["a"],"sources":["metadata","terraform"]}`},
		{"tf allow_null preserved across metadata range", cty.Number,
			`var.x == null ? true : (var.x >= 0 && var.x <= 10)`,
			ValidationRule{Validator: "range", Inputs: map[string]interface{}{"min": 0, "max": 10}},
			`{"min":0,"max":10,"allow_null":true,"sources":["metadata","terraform"]}`},
		{"tf unparsed null-guard recovers allow_null for metadata range", cty.Number,
			`var.x == null || try(var.x >= 0, false)`,
			ValidationRule{Validator: "range", Inputs: map[string]interface{}{"min": 0, "max": 2}},
			`{"min":0,"max":2,"allow_null":true,"sources":["metadata"]}`},
		{"tf allow_null preserved across metadata regex", cty.String,
			`var.x == null || can(regex("^[a-z]+$", var.x))`,
			ValidationRule{Validator: "regex", Inputs: map[string]interface{}{"pattern": "^[a-z]+$"}},
			`{"pattern":"^[a-z]+$","allow_null":true,"sources":["metadata","terraform"]}`},
		{"tf rejects null despite metadata regex pass-through", cty.String,
			`var.x != null && can(regex("^[a-z]+$", var.x))`,
			ValidationRule{Validator: "regex", Inputs: map[string]interface{}{"pattern": "^[a-z]+$"}},
			`{"pattern":"^[a-z]+$","sources":["metadata","terraform"]}`},
		{"metadata required suppresses tf allow_null", cty.String,
			`var.x == null || contains(["a", "b"], var.x)`,
			ValidationRule{Validator: "required", Inputs: map[string]interface{}{}},
			`{"allowed_values":["a","b"],"sources":["terraform"]}`},
		{"metadata cidr without allow_null suppresses tf allow_null", cty.String,
			`var.x == null || can(regex("/28$", var.x))`,
			ValidationRule{Validator: "cidr", Inputs: map[string]interface{}{}},
			`{"pattern":"/28$","sources":["terraform"]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := constraintsJSON(t, buildInputConstraints(tc.typ, meta(tc.typ, tc.meta), conditionRules(t, tc.tf)))
			if got != tc.want {
				t.Errorf("want: %s\ngot:  %s", tc.want, got)
			}
		})
	}
}

func TestApplyVarMetaConstraintsAndMetadataMessages(t *testing.T) {
	dir := t.TempDir()
	src := `
variable "tier" {
  type    = string
  default = null
  validation {
    condition     = var.tier == null || contains(["BASIC", "ZONAL"], var.tier)
    error_message = "tier must be BASIC or ZONAL."
  }
}
variable "strict_tier" {
  type     = string
  nullable = false
}
variable "two_blocks" {
  type = string
  validation {
    condition     = var.two_blocks == null || contains(["a", "b"], var.two_blocks)
    error_message = "a or b."
  }
  validation {
    condition     = length(var.two_blocks) > 0
    error_message = "non-empty."
  }
}
variable "null_guarded_meta" {
  type    = number
  default = null
  validation {
    condition     = var.null_guarded_meta == null || try(var.null_guarded_meta >= 0, false)
    error_message = "0..2 or null."
  }
}
variable "zones" {
  type    = list(string)
  default = []
  validation {
    condition     = alltrue([for z in var.zones : can(regex("^us-", z))])
    error_message = "zones must be in the US."
  }
}
variable "legacy" {
  type    = string
  default = null
  validation {
    condition     = var.legacy == null
    error_message = "legacy is deprecated."
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	list := func(items ...interface{}) []interface{} { return items }
	orig := ModuleInfo{
		Inputs: []VarInfo{
			{Name: "tier", Type: cty.String},
			{Name: "strict_tier", Type: cty.String},
			{Name: "two_blocks", Type: cty.String},
			{Name: "null_guarded_meta", Type: cty.Number},
			{Name: "zones", Type: cty.List(cty.String)},
			{Name: "legacy", Type: cty.String},
			{Name: "size_gb", Type: cty.Number},
		},
		Metadata: Metadata{Ghpc: MetadataGhpc{Validators: []ValidationRule{
			{Validator: "allowed_enum", ErrorMessage: "tier must be one of BASIC, ZONAL or REGIONAL.",
				Inputs: map[string]interface{}{"vars": list("tier"), "allowed": list("BASIC", "ZONAL", "REGIONAL"), "allow_null": true}},
			{Validator: "allowed_enum", ErrorMessage: "strict_tier must be A or B.",
				Inputs: map[string]interface{}{"vars": list("strict_tier"), "allowed": list("A", "B"), "allow_null": true}},
			{Validator: "range", ErrorMessage: "0..2.",
				Inputs: map[string]interface{}{"vars": list("null_guarded_meta"), "min": 0, "max": 2}},
			{Validator: "range", ErrorMessage: "size_gb must be at least 1024.",
				Inputs: map[string]interface{}{"vars": list("size_gb"), "min": 1024}},
		}}},
	}

	mi := ApplyVarMeta(dir, orig)
	if orig.Inputs[0].Constraints != nil || orig.Inputs[0].Validations != nil {
		t.Fatalf("expected ApplyVarMeta not to mutate the original inputs, got %+v", orig.Inputs[0])
	}

	wants := []struct {
		constraints string
		validations []string
		deprecated  bool
	}{
		{`{"allowed_values":["BASIC","ZONAL"],"allow_null":true,"sources":["metadata","terraform"]}`,
			[]string{"tier must be BASIC or ZONAL.", "tier must be one of BASIC, ZONAL or REGIONAL."}, false},
		{`{"allowed_values":["A","B"],"sources":["metadata"]}`,
			[]string{"strict_tier must be A or B."}, false},
		{`{"allowed_values":["a","b"],"sources":["terraform"]}`,
			[]string{"a or b.", "non-empty."}, false},
		{`{"min":0,"max":2,"allow_null":true,"sources":["metadata"]}`,
			[]string{"0..2 or null.", "0..2."}, false},
		{`null`, []string{"zones must be in the US."}, false},
		{`null`, nil, true},
		{`{"min":1024,"sources":["metadata"]}`, []string{"size_gb must be at least 1024."}, false},
	}
	for i, w := range wants {
		in := mi.Inputs[i]
		if got := constraintsJSON(t, in.Constraints); got != w.constraints {
			t.Errorf("%s constraints\nwant: %s\ngot:  %s", in.Name, w.constraints, got)
		}
		if !slices.Equal(in.Validations, w.validations) {
			t.Errorf("%s validations: want %q, got %q", in.Name, w.validations, in.Validations)
		}
		if in.Deprecated != w.deprecated {
			t.Errorf("%s deprecated: want %v, got %v", in.Name, w.deprecated, in.Deprecated)
		}
	}
}
