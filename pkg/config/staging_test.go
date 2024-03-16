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
	"path/filepath"
	"strings"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/exp/slices"
	. "gopkg.in/check.v1"
)

func (s *zeroSuite) TestValidateNoGhpcStageFuncs(c *C) {
	bp := Blueprint{
		Groups: []Group{{
			Modules: []Module{
				{
					Settings: Dict{}.
						With("tree", MustParseExpression("ghpc_stage(\"bush\")").AsValue()),
				}}}}}
	c.Check(bp.validateNoGhpcStageFuncs(), NotNil)
}

func (s *zeroSuite) TestGhpcStageImpl(c *C) {
	h := func(path, want string) {
		bp := Blueprint{path: "/zebra/greendoodle.yaml"}
		c.Check(bp.makeGhpcStageImpl()(path), Equals, want)
		c.Check(bp.StagedFiles(), DeepEquals, []StagedFile{
			{AbsSrc: filepath.Join("/zebra/", path), RelDst: want},
		})
	}

	h("zero", "../.ghpc/staged/zero_d02c4c4cde")
	h("zero/one.txt", "../.ghpc/staged/one.txt_f8669c6c22")
	h("./../../two.gif", "../.ghpc/staged/two.gif_711b257c4f")
	h(".", "../.ghpc/staged/file_5058f1af83")
	h("..", "../.ghpc/staged/file_58b9e70b65")

	{
		bp := Blueprint{path: "/zebra/greendoodle.yaml"}

		c.Check(bp.makeGhpcStageImpl()("one.txt"), Equals, "../.ghpc/staged/one.txt_08bc3de154")
		c.Check(bp.makeGhpcStageImpl()("zero/one.txt"), Equals, "../.ghpc/staged/one.txt_f8669c6c22")
		c.Check(bp.makeGhpcStageImpl()("/root/abs.txt"), Equals, "../.ghpc/staged/abs.txt_ffac5d1d6b")

		got := bp.StagedFiles()
		slices.SortFunc(got, func(a, b StagedFile) int {
			return strings.Compare(a.AbsSrc, b.AbsSrc)
		})

		if diff := cmp.Diff(got, []StagedFile{
			{"/root/abs.txt", "../.ghpc/staged/abs.txt_ffac5d1d6b"},
			{"/zebra/one.txt", "../.ghpc/staged/one.txt_08bc3de154"},
			{"/zebra/zero/one.txt", "../.ghpc/staged/one.txt_f8669c6c22"},
		}); diff != "" {
			c.Errorf("diff (-want +got):\n%s", diff)
		}
	}
}

func (s *zeroSuite) TestGhpcStageFunc(c *C) {
	bp := Blueprint{path: "/zebra/greendoodle.yaml"}

	h := func(p string) string {
		g, err := bp.Eval(MustParseExpression("ghpc_stage(\"" + p + "\")").AsValue())
		if err != nil {
			c.Fatal(err)
		}
		return g.AsString()
	}

	c.Check(h("bush"), Equals, "../.ghpc/staged/bush_dbbc546e35")
	c.Check(h("push"), Equals, "../.ghpc/staged/push_21a361d96e")

	got := bp.StagedFiles()
	slices.SortFunc(got, func(a, b StagedFile) int {
		return strings.Compare(a.AbsSrc, b.AbsSrc)
	})

	if diff := cmp.Diff(got, []StagedFile{
		{"/zebra/bush", "../.ghpc/staged/bush_dbbc546e35"},
		{"/zebra/push", "../.ghpc/staged/push_21a361d96e"},
	}); diff != "" {
		c.Errorf("diff (-want +got):\n%s", diff)
	}
}
