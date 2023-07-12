// Copyright 2023 Google LLC
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

package cmd

import (
	"errors"
	"hpc-toolkit/pkg/config"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

func (s *MySuite) TestSetCLIVariables(c *C) {
	bp := config.Blueprint{}
	bp.Vars.Set("deployment_name", cty.StringVal("bush"))

	vars := []string{
		"project_id=cli_test_project_id",
		"deployment_name=cli_deployment_name",
		"region=cli_region",
		"zone=cli_zone",
		"kv=key=val",
		"keyBool=true",
		"keyInt=15",
		"keyFloat=15.43",
		"keyMap={bar: baz, qux: 1}",
		"keyArray=[1, 2, 3]",
		"keyArrayOfMaps=[foo, {bar: baz, qux: 1}]",
		"keyMapOfArrays={foo: [1, 2, 3], bar: [a, b, c]}",
	}
	c.Assert(setCLIVariables(&bp, vars), IsNil)
	c.Check(
		bp.Vars.Items(), DeepEquals, map[string]cty.Value{
			"project_id":      cty.StringVal("cli_test_project_id"),
			"deployment_name": cty.StringVal("cli_deployment_name"),
			"region":          cty.StringVal("cli_region"),
			"zone":            cty.StringVal("cli_zone"),
			"kv":              cty.StringVal("key=val"),
			"keyBool":         cty.True,
			"keyInt":          cty.NumberIntVal(15),
			"keyFloat":        cty.NumberFloatVal(15.43),
			"keyMap": cty.ObjectVal(map[string]cty.Value{
				"bar": cty.StringVal("baz"),
				"qux": cty.NumberIntVal(1)}),
			"keyArray": cty.TupleVal([]cty.Value{
				cty.NumberIntVal(1), cty.NumberIntVal(2), cty.NumberIntVal(3)}),
			"keyArrayOfMaps": cty.TupleVal([]cty.Value{
				cty.StringVal("foo"),
				cty.ObjectVal(map[string]cty.Value{
					"bar": cty.StringVal("baz"),
					"qux": cty.NumberIntVal(1)})}),
			"keyMapOfArrays": cty.ObjectVal(map[string]cty.Value{
				"foo": cty.TupleVal([]cty.Value{
					cty.NumberIntVal(1), cty.NumberIntVal(2), cty.NumberIntVal(3)}),
				"bar": cty.TupleVal([]cty.Value{
					cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c")}),
			}),
		})

	// Failure: Variable without '='
	bp = config.Blueprint{}
	inv := []string{"project_idcli_test_project_id"}

	c.Assert(setCLIVariables(&bp, inv), ErrorMatches, "invalid format: .*")
	c.Check(bp.Vars, DeepEquals, config.Dict{})
}

func (s *MySuite) TestSetBackendConfig(c *C) {
	// Success
	vars := []string{
		"taste=sweet",
		"type=green",
		"odor=strong",
	}

	bp := config.Blueprint{}
	c.Assert(setBackendConfig(&bp, vars), IsNil)

	be := bp.TerraformBackendDefaults
	c.Check(be.Type, Equals, "green")
	c.Check(be.Configuration.Items(), DeepEquals, map[string]cty.Value{
		"taste": cty.StringVal("sweet"),
		"odor":  cty.StringVal("strong"),
	})
}

func (s *MySuite) TestSetBackendConfig_Invalid(c *C) {
	// Failure: Variable without '='
	vars := []string{
		"typegreen",
	}
	bp := config.Blueprint{}
	c.Assert(setBackendConfig(&bp, vars), ErrorMatches, "invalid format: .*")
}

func (s *MySuite) TestSetBackendConfig_NoOp(c *C) {
	bp := config.Blueprint{
		TerraformBackendDefaults: config.TerraformBackend{
			Type: "green"}}

	c.Assert(setBackendConfig(&bp, []string{}), IsNil)
	c.Check(bp.TerraformBackendDefaults, DeepEquals, config.TerraformBackend{
		Type: "green"})
}

func (s *MySuite) TestValidationLevels(c *C) {
	bp := config.Blueprint{}

	c.Check(setValidationLevel(&bp, "ERROR"), IsNil)
	c.Check(bp.ValidationLevel, Equals, config.ValidationError)

	c.Check(setValidationLevel(&bp, "WARNING"), IsNil)
	c.Check(bp.ValidationLevel, Equals, config.ValidationWarning)

	c.Check(setValidationLevel(&bp, "IGNORE"), IsNil)
	c.Check(bp.ValidationLevel, Equals, config.ValidationIgnore)

	c.Check(setValidationLevel(&bp, "INVALID"), NotNil)
}

func (s *MySuite) TestRenderError(c *C) {
	{ // simple
		err := errors.New("arbuz")
		got := renderError(err, config.YamlCtx{})
		c.Check(got, Equals, "arbuz")
	}
	{ // has pos, but context is missing
		ctx := config.NewYamlCtx([]byte(``))
		pth := config.Root.Vars.Dot("kale")
		err := config.BpError{Path: pth, Err: errors.New("arbuz")}
		got := renderError(err, ctx)
		c.Check(got, Equals, "vars.kale: arbuz")
	}
	{ // has pos, has context
		ctx := config.NewYamlCtx([]byte(`
vars:
  kale: dos`))
		pth := config.Root.Vars.Dot("kale")
		err := config.BpError{Path: pth, Err: errors.New("arbuz")}
		got := renderError(err, ctx)
		c.Check(got, Equals, `
Error: arbuz
3:   kale: dos
           ^`)
	}
}
