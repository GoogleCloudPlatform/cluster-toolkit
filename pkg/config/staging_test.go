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
	"path/filepath"
	"strings"

	"testing"

	"golang.org/x/exp/slices"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	. "gopkg.in/check.v1"
)

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

func TestPartialEval(t *testing.T) {
	type test struct {
		input string
		want  string
	}

	// testing partial evaluation of `upper`
	tests := []test{
		{`upper("a")`, `"A"`},
		{`file(upper("a"))`, `file("A")`},
		{`lower("A") + upper("b")`, `lower("A")+"B"`},
		{`upper(lower("A"))`, `"A"`},
		{`upper("hello ${upper("world")}") + 7`, `"HELLO WORLD"+7`},
		{`upper("a") + upper("b")`, `"A"+"B"`},
		{`"hello ${upper("World")}"`, `"hello ${"WORLD"}"`},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			in := MustParseExpression(tc.input)
			ctx := hcl.EvalContext{Functions: map[string]function.Function{
				"upper": stdlib.UpperFunc,
				"lower": stdlib.LowerFunc,
			}}

			got, err := partialEval(in, "upper", &ctx)
			if err != nil {
				t.Errorf("got unexpected error: %s", err)
			}

			if diff := cmp.Diff(tc.want, string(got.Tokenize().Bytes())); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEvalGhpcStage(t *testing.T) {
	mod := Module{
		Settings: Dict{}.
			With("war", MustParseExpression(`never("changes")`).AsValue()).
			With("aqua", MustParseExpression(`ghpc_stage("cola")`).AsValue()).
			With("guzz", MustParseExpression(`"${ghpc_stage("oline")}/hello.sh"`).AsValue()),
	}
	bp := Blueprint{
		path:   "/zebra/greendoodle.yaml",
		Groups: []Group{{Modules: []Module{mod}}},
	}

	if err := bp.evalGhpcStage(); err != nil {
		t.Errorf("got unexpected error: %v", err)
	}

	updated := bp.Groups[0].Modules[0].Settings
	{ // No changes
		want := `never("changes")`
		got := string(TokensForValue(updated.Get("war")).Bytes())
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("diff (-want +got):\n%s", diff)
		}
	}
	{ // Simple case
		want := `"../.ghpc/staged/cola_a1e05ee256"`
		got := string(TokensForValue(updated.Get("aqua")).Bytes())

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("diff (-want +got):\n%s", diff)
		}
	}
	{ // Partial evaluation
		want := `"${"../.ghpc/staged/oline_99878776f5"}/hello.sh"`
		got := string(TokensForValue(updated.Get("guzz")).Bytes())

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("diff (-want +got):\n%s", diff)
		}
	}
	{ // check that bp.stageFiles are updated
		want := map[string]string{
			"cola":  "../.ghpc/staged/cola_a1e05ee256",
			"oline": "../.ghpc/staged/oline_99878776f5",
		}
		if diff := cmp.Diff(want, bp.stagedFiles); diff != "" {
			t.Errorf("diff (-want +got):\n%s", diff)
		}
	}
}
