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

package config

import (
	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

type MaterializeSuite struct{}

var _ = Suite(&MaterializeSuite{})

func (s *MaterializeSuite) TestMaterializeSuccess(c *C) {
	bp := Blueprint{
		BlueprintName: "test-bp",
		Vars: NewDict(map[string]cty.Value{
			"project": cty.StringVal("test-project"),
		}),
		Groups: []Group{
			{
				Name: "group1",
				TerraformBackend: TerraformBackend{
					Type: "gcs",
					Configuration: NewDict(map[string]cty.Value{
						"bucket": cty.StringVal("test-bucket"),
					}),
				},
			},
		},
	}

	err := bp.Materialize()
	c.Assert(err, IsNil)
}
