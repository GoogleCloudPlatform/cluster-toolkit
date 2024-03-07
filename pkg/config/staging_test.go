// Copyright 2024 "Google LLC"
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
	. "gopkg.in/check.v1"
)

func (s *zeroSuite) TestValidateNoGhpcStageFuncs(c *C) {
	bp := Blueprint{
		DeploymentGroups: []DeploymentGroup{{
			Modules: []Module{
				{
					Settings: Dict{}.
						With("tree", MustParseExpression("ghpc_stage(\"bush\")").AsValue()),
				}}}}}
	c.Check(bp.validateNoGhpcStageFuncs(), NotNil)
}

func (s *zeroSuite) TestGhpcStageImpl(c *C) {
	h := func(path, want string) {
		bp := Blueprint{}
		c.Check(bp.makeGhpcStageImpl()(path), Equals, want)
		c.Check(bp.StagedFiles(), DeepEquals, map[string]string{path: want})
	}

	h("zero", "../.ghpc/staged/zero_d02c4c4cde")
	h("zero/one.txt", "../.ghpc/staged/one.txt_f8669c6c22")
	h("./../../two.gif", "../.ghpc/staged/two.gif_711b257c4f")
	h(".", "../.ghpc/staged/file_5058f1af83")
	h("..", "../.ghpc/staged/file_58b9e70b65")
	h("/", "../.ghpc/staged/file_6666cd76f9")

	{
		bp := Blueprint{}
		wantOne := "../.ghpc/staged/one.txt_08bc3de154"
		wantZeroOne := "../.ghpc/staged/one.txt_f8669c6c22"

		c.Check(bp.makeGhpcStageImpl()("one.txt"), Equals, wantOne)
		c.Check(bp.makeGhpcStageImpl()("zero/one.txt"), Equals, wantZeroOne)

		c.Check(bp.StagedFiles(), DeepEquals, map[string]string{
			"zero/one.txt": wantZeroOne,
			"one.txt":      wantOne,
		})
	}

}

func (s *zeroSuite) TestGhpcStageFunc(c *C) {
	bp := Blueprint{}

	h := func(p string) string {
		g, err := bp.Eval(MustParseExpression("ghpc_stage(\"" + p + "\")").AsValue())
		if err != nil {
			c.Fatal(err)
		}
		return g.AsString()
	}

	c.Check(h("bush"), Equals, "../.ghpc/staged/bush_dbbc546e35")
	c.Check(h("push"), Equals, "../.ghpc/staged/push_21a361d96e")
	c.Check(bp.StagedFiles(), DeepEquals, map[string]string{
		"bush": "../.ghpc/staged/bush_dbbc546e35",
		"push": "../.ghpc/staged/push_21a361d96e",
	})
}
