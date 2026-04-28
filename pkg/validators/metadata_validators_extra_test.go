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
	"strings"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

type MetadataValidatorsExtraSuite struct{}

var _ = Suite(&MetadataValidatorsExtraSuite{})

func (s *MetadataValidatorsExtraSuite) TestRegexValidatorErrors(c *C) {
	bp := config.Blueprint{
		Groups: []config.Group{{Name: "g1", Modules: []config.Module{{ID: "m1"}}}},
	}
	v := RegexValidator{}

	// Missing pattern
	rule := modulereader.ValidationRule{Inputs: map[string]interface{}{}}
	err := v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "missing a string 'pattern'"), Equals, true)

	// Invalid pattern
	rule = modulereader.ValidationRule{Inputs: map[string]interface{}{"pattern": "[["}}
	err = v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "failed to compile regex"), Equals, true)
}

func (s *MetadataValidatorsExtraSuite) TestAllowedEnumValidatorErrors(c *C) {
	bp := config.Blueprint{
		Groups: []config.Group{{Name: "g1", Modules: []config.Module{{ID: "m1"}}}},
	}
	v := AllowedEnumValidator{}

	// Missing allowed
	rule := modulereader.ValidationRule{Inputs: map[string]interface{}{}}
	err := v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "missing an 'allowed' list"), Equals, true)

	// Invalid allowed type
	rule = modulereader.ValidationRule{Inputs: map[string]interface{}{"allowed": "not a list"}}
	err = v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "'allowed' must be a list"), Equals, true)

	// Empty allowed list
	rule = modulereader.ValidationRule{Inputs: map[string]interface{}{"allowed": []string{}}}
	err = v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "'allowed' list must be non-empty"), Equals, true)

	// checkValues null not allowed
	rule = modulereader.ValidationRule{
		Inputs: map[string]interface{}{
			"allowed":    []string{"foo"},
			"vars":       []string{"v1"},
			"allow_null": false,
		},
	}
	bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{"v1": cty.NullVal(cty.String)})
	err = v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "null value is not allowed"), Equals, true)
}

func (s *MetadataValidatorsExtraSuite) TestRangeValidatorErrors(c *C) {
	bp := config.Blueprint{
		Groups: []config.Group{{Name: "g1", Modules: []config.Module{{ID: "m1"}}}},
	}
	v := RangeValidator{}

	// Both min and max missing
	rule := modulereader.ValidationRule{Inputs: map[string]interface{}{}}
	err := v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "must have at least one of 'min' or 'max'"), Equals, true)

	// min > max
	rule = modulereader.ValidationRule{Inputs: map[string]interface{}{"min": 10, "max": 5}}
	err = v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "must have 'min' less than or equal to 'max'"), Equals, true)

	// Range validator with non-integer number
	rule = modulereader.ValidationRule{Inputs: map[string]interface{}{"min": 0, "vars": []string{"v1"}}}
	bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{"v1": cty.NumberFloatVal(1.5)})
	err = v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "only supports integer numbers"), Equals, true)

	// Range validator with non-number
	bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{"v1": cty.StringVal("foo")})
	err = v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "only supports numbers, not string"), Equals, true)
}

func (s *MetadataValidatorsExtraSuite) TestRequiredValidatorErrors(c *C) {
	bp := config.Blueprint{
		Groups: []config.Group{{Name: "g1", Modules: []config.Module{{ID: "m1"}}}},
	}
	v := RequiredValidator{}

	// Missing vars
	rule := modulereader.ValidationRule{Inputs: map[string]interface{}{}}
	err := v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "missing 'vars'"), Equals, true)
}

func (s *MetadataValidatorsExtraSuite) TestConditionalValidatorErrors(c *C) {
	bp := config.Blueprint{
		Groups: []config.Group{{Name: "g1", Modules: []config.Module{{ID: "m1"}}}},
	}
	v := ConditionalValidator{}

	// Missing trigger
	rule := modulereader.ValidationRule{Inputs: map[string]interface{}{}}
	err := v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "missing 'trigger'"), Equals, true)

	// Missing dependent
	rule = modulereader.ValidationRule{Inputs: map[string]interface{}{"trigger": "t1"}}
	bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{"t1": cty.BoolVal(true)})
	err = v.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "missing 'dependent'"), Equals, true)
}
