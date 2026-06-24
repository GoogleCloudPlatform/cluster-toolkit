// Copyright 2026 "Google LLC"
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

package validators

import (
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

type MetadataHelpersSuite struct{}

var _ = Suite(&MetadataHelpersSuite{})

func (s *MetadataHelpersSuite) TestGetNestedValue(c *C) {
	d := config.NewDict(map[string]cty.Value{
		"a": cty.ObjectVal(map[string]cty.Value{
			"b": cty.StringVal("foo"),
		}),
		"c": cty.StringVal("bar"),
	})

	val, ok := getNestedValue(d, "a.b")
	c.Assert(ok, Equals, true)
	c.Assert(val.AsString(), Equals, "foo")

	_, ok = getNestedValue(d, "a.c")
	c.Assert(ok, Equals, false)

	_, ok = getNestedValue(d, "c.d")
	c.Assert(ok, Equals, false)

	_, ok = getNestedValue(d, "nonexistent")
	c.Assert(ok, Equals, false)
}

func (s *MetadataHelpersSuite) TestParseStringList(c *C) {
	res, ok := parseStringList("foo")
	c.Assert(ok, Equals, true)
	c.Assert(res, DeepEquals, []string{"foo"})

	res, ok = parseStringList([]interface{}{"foo", "bar"})
	c.Assert(ok, Equals, true)
	c.Assert(res, DeepEquals, []string{"foo", "bar"})

	res, ok = parseStringList([]string{"foo", "bar"})
	c.Assert(ok, Equals, true)
	c.Assert(res, DeepEquals, []string{"foo", "bar"})

	_, ok = parseStringList(nil)
	c.Assert(ok, Equals, false)

	_, ok = parseStringList(123)
	c.Assert(ok, Equals, false)

	_, ok = parseStringList([]interface{}{"foo", 123})
	c.Assert(ok, Equals, false)
}

func (s *MetadataHelpersSuite) TestParseString(c *C) {
	res, ok := parseString("foo")
	c.Assert(ok, Equals, true)
	c.Assert(res, Equals, "foo")

	res, ok = parseString([]interface{}{"foo"})
	c.Assert(ok, Equals, true)
	c.Assert(res, Equals, "foo")

	res, ok = parseString([]string{"foo"})
	c.Assert(ok, Equals, true)
	c.Assert(res, Equals, "foo")

	_, ok = parseString(nil)
	c.Assert(ok, Equals, false)

	_, ok = parseString([]interface{}{"foo", "bar"})
	c.Assert(ok, Equals, false)

	_, ok = parseString(123)
	c.Assert(ok, Equals, false)
}

func (s *MetadataHelpersSuite) TestParseIntInput(c *C) {
	inputs := map[string]interface{}{
		"a": 1,
		"b": 2.0,
		"c": "not an int",
	}

	res, err := parseIntInput(inputs, "a")
	c.Assert(err, IsNil)
	c.Assert(*res, Equals, 1)

	res, err = parseIntInput(inputs, "b")
	c.Assert(err, IsNil)
	c.Assert(*res, Equals, 2)

	_, err = parseIntInput(inputs, "c")
	c.Assert(err, NotNil)

	res, err = parseIntInput(inputs, "nonexistent")
	c.Assert(err, IsNil)
	c.Assert(res, IsNil)
}

func (s *MetadataHelpersSuite) TestConvertToCty(c *C) {
	c.Assert(convertToCty(nil), Equals, cty.NilVal)
	c.Assert(convertToCty(true).RawEquals(cty.BoolVal(true)), Equals, true)
	c.Assert(convertToCty(1).RawEquals(cty.NumberIntVal(1)), Equals, true)
	c.Assert(convertToCty(1.5).RawEquals(cty.NumberFloatVal(1.5)), Equals, true)
	c.Assert(convertToCty("foo").RawEquals(cty.StringVal("foo")), Equals, true)

	res := convertToCty([]interface{}{"foo", 1})
	c.Assert(res.Type().IsTupleType(), Equals, true)

	res = convertToCty(map[string]interface{}{"foo": "bar"})
	c.Assert(res.Type().IsObjectType(), Equals, true)

	c.Assert(convertToCty(struct{}{}), Equals, cty.NilVal)
}

func (s *MetadataHelpersSuite) TestFormatValue(c *C) {
	c.Assert(formatValue(nil), Equals, "null")
	c.Assert(formatValue([]cty.Value{cty.NullVal(cty.String)}), Equals, "null")
	c.Assert(formatValue([]cty.Value{cty.StringVal("foo")}), Equals, "\"foo\"")
}

func (s *MetadataHelpersSuite) TestIsVarSet(c *C) {
	c.Assert(isVarSet(nil), Equals, false)
	c.Assert(isVarSet([]cty.Value{cty.StringVal("")}), Equals, false)
	c.Assert(isVarSet([]cty.Value{cty.NumberIntVal(0)}), Equals, false)
	c.Assert(isVarSet([]cty.Value{cty.BoolVal(false)}), Equals, false)
	c.Assert(isVarSet([]cty.Value{cty.ListValEmpty(cty.String)}), Equals, false)
	c.Assert(isVarSet([]cty.Value{cty.StringVal("foo")}), Equals, true)
}

func (s *MetadataHelpersSuite) TestIterateRuleTargets(c *C) {
	bp := config.Blueprint{}
	mod := config.Module{
		ID: "m1",
		Settings: config.NewDict(map[string]cty.Value{
			"var_a": cty.StringVal("val_a"),
		}),
	}
	rule := modulereader.ValidationRule{
		Inputs: map[string]interface{}{
			"vars":     []interface{}{"var_a"},
			"optional": false,
		},
	}
	group := config.Group{Modules: []config.Module{mod}}

	var called bool
	err := IterateRuleTargets(bp, mod, rule, group, 0, func(t Target) error {
		called = true
		c.Assert(t.Name, Equals, "var_a")
		c.Assert(t.Values[0].AsString(), Equals, "val_a")
		return nil
	})
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
}
