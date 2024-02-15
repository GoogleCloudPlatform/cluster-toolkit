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

package cmd

import (
	"errors"
	"hpc-toolkit/pkg/config"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func makeCtx(yml string, t *testing.T) config.YamlCtx {
	ctx, err := config.NewYamlCtx([]byte(yml))
	if err != nil {
		t.Fatal(err, yml)
	}
	return ctx
}

func TestRenderError(t *testing.T) {
	type test struct {
		err  error
		ctx  config.YamlCtx
		want string
	}
	tests := []test{
		{errors.New("arbuz"), makeCtx("", t), "Error: arbuz"},
		{ // has pos, but context doesn't contain it
			err:  config.BpError{Path: config.Root.Vars.Dot("kale"), Err: errors.New("arbuz")},
			ctx:  makeCtx("", t),
			want: "Error: arbuz"},
		{ // has pos, has context
			err: config.BpError{Path: config.Root.Vars.Dot("kale"), Err: errors.New("arbuz")},
			ctx: makeCtx(`
vars:
  kale: dos`, t),
			want: `Error: arbuz
3:   kale: dos
     ^`},
		{
			err: config.HintError{Hint: "did you mean 'kale'?", Err: errors.New("arbuz")},
			ctx: makeCtx("", t),
			want: `Error: arbuz
Hint: did you mean 'kale'?`},
		{ // has pos, has context
			err: config.BpError{
				Path: config.Root.Vars.Dot("kale"),
				Err: config.HintError{
					Hint: "did you mean 'kale'?",
					Err:  errors.New("arbuz")}},
			ctx: makeCtx(`
vars:
  kale: dos`, t),
			want: `Error: arbuz
Hint: did you mean 'kale'?
3:   kale: dos
     ^`},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := renderError(tc.err, tc.ctx)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}
