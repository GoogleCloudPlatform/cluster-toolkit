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

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

type SemanticSuite struct{}

var _ = Suite(&SemanticSuite{})

func (s *SemanticSuite) TestTestModuleNotUsed(c *C) {
	bp := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "group1",
				Modules: []config.Module{
					{
						ID:   "module1",
						Kind: config.TerraformKind,
					},
					{
						ID:   "module2",
						Kind: config.TerraformKind,
						Use:  config.ModuleIDs{"module1"},
					},
				},
			},
		},
	}

	inputs := config.NewDict(map[string]cty.Value{})
	err := testModuleNotUsed(bp, inputs)
	c.Assert(err, NotNil)
}

func (s *SemanticSuite) TestTestModuleUsed(c *C) {
	bp := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "group1",
				Modules: []config.Module{
					{
						ID:   "module1",
						Kind: config.TerraformKind,
					},
					{
						ID:   "module2",
						Kind: config.TerraformKind,
						Use:  config.ModuleIDs{"module1"},
						Settings: config.NewDict(map[string]cty.Value{
							"setting": config.AsProductOfModuleUse(cty.StringVal("some-value"), "module1"),
						}),
					},
				},
			},
		},
	}

	inputs := config.NewDict(map[string]cty.Value{})
	err := testModuleNotUsed(bp, inputs)
	c.Assert(err, IsNil)
}

func (s *SemanticSuite) TestTestDeploymentVariableNotUsed(c *C) {
	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"var1": cty.StringVal("val1"),
		}),
	}

	inputs := config.NewDict(map[string]cty.Value{})
	err := testDeploymentVariableNotUsed(bp, inputs)
	c.Assert(err, NotNil) // var1 is not used
}
