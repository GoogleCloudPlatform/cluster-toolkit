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

package validators

import (
	"hpc-toolkit/pkg/config"
	"testing"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *MySuite) TestCheckInputs(c *C) {
	dummy := cty.NullVal(cty.String)

	{ // OK: Inputs is equal to required inputs without regard to ordering
		i := config.NewDict(map[string]cty.Value{
			"in0": dummy,
			"in1": dummy})
		c.Check(checkInputs(i, []string{"in0", "in1"}), IsNil)
		c.Check(checkInputs(i, []string{"in1", "in0"}), IsNil)
	}

	{ // FAIL: inputs are a proper subset of required inputs
		i := config.NewDict(map[string]cty.Value{
			"in0": dummy,
			"in1": dummy})
		err := checkInputs(i, []string{"in0", "in1", "in2"})
		c.Check(err, NotNil)
	}

	{ // FAIL: inputs intersect with required inputs but are not a proper subset
		i := config.NewDict(map[string]cty.Value{
			"in0": dummy,
			"in1": dummy,
			"in3": dummy})
		err := checkInputs(i, []string{"in0", "in1", "in2"})
		c.Check(err, NotNil)
	}

	{ // FAIL inputs are a proper superset of required inputs
		i := config.NewDict(map[string]cty.Value{
			"in0": dummy,
			"in1": dummy,
			"in2": dummy,
			"in3": dummy})
		err := checkInputs(i, []string{"in0", "in1", "in2"})
		c.Check(err, ErrorMatches, "only 3 inputs \\[in0 in1 in2\\] should be provided")
	}
}

func (s *MySuite) TestDefaultValidators(c *C) {
	unusedMods := config.Validator{Validator: "test_module_not_used"}
	unusedVars := config.Validator{Validator: "test_deployment_variable_not_used"}
	slurmTf := config.Validator{Validator: "test_tf_version_for_slurm"}

	prjInp := config.Dict{}.With("project_id", config.GlobalRef("project_id").AsValue())
	regInp := prjInp.With("region", config.GlobalRef("region").AsValue())
	zoneInp := prjInp.With("zone", config.GlobalRef("zone").AsValue())
	regZoneInp := regInp.With("zone", config.GlobalRef("zone").AsValue())

	projectExists := config.Validator{
		Validator: "test_project_exists", Inputs: prjInp}
	apisEnabled := config.Validator{
		Validator: "test_apis_enabled", Inputs: prjInp}
	regionExists := config.Validator{
		Validator: testRegionExistsName, Inputs: regInp}
	zoneExists := config.Validator{
		Validator: testZoneExistsName, Inputs: zoneInp}
	zoneInRegion := config.Validator{
		Validator: testZoneInRegionName, Inputs: regZoneInp}

	{
		bp := config.Blueprint{}
		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, slurmTf})
	}

	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b"))}
		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, slurmTf, projectExists, apisEnabled})
	}

	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b")).
			With("region", cty.StringVal("narnia"))}

		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, slurmTf, projectExists, apisEnabled, regionExists})
	}

	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b")).
			With("zone", cty.StringVal("danger"))}

		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, slurmTf, projectExists, apisEnabled, zoneExists})
	}

	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b")).
			With("region", cty.StringVal("narnia")).
			With("zone", cty.StringVal("danger"))}

		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, slurmTf, projectExists, apisEnabled, regionExists, zoneExists, zoneInRegion})
	}
}
