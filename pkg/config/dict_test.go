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

package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
)

func TestZeroValueValid(t *testing.T) {
	{ // Items
		d := Dict{}
		want := map[string]cty.Value{}
		got := d.Items()
		if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
			t.Errorf("diff (-want +got):\n%s", diff)
		}
	}

	{ // Has
		d := Dict{}
		if d.Has("zebra") != false {
			t.Errorf("should not contain any values")
		}
	}

	{ // Get
		d := Dict{}
		want := cty.NilVal
		got := d.Get("pony")
		if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
			t.Errorf("diff (-want +got):\n%s", diff)
		}
	}
}

func TestSetAndGet(t *testing.T) {
	want := cty.StringVal("guava")
	d := Dict{}.With("hare", want)
	got := d.Get("hare")
	if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
	if !d.Has("hare") {
		t.Errorf("should have a hare")
	}
}

func TestItemsAreCopy(t *testing.T) {
	d := Dict{}.With("apple", cty.StringVal("fuji"))

	items := d.Items()
	items["apple"] = cty.StringVal("opal")

	want := cty.StringVal("fuji")
	got := d.Get("apple")
	if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestEval(t *testing.T) {
	bp := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"zebra": cty.StringVal("stripes"),
		}),
	}
	d := NewDict(map[string]cty.Value{
		"abyss": cty.ObjectVal(map[string]cty.Value{
			"white": GlobalRef("zebra").AsValue(),
			"green": cty.StringVal("grass"),
		})})
	want := NewDict(map[string]cty.Value{
		"abyss": cty.ObjectVal(map[string]cty.Value{
			"white": cty.StringVal("stripes"),
			"green": cty.StringVal("grass"),
		})})
	got, err := d.Eval(bp)
	if err != nil {
		t.Fatalf("failed to eval: %v", err)
	}
	if diff := cmp.Diff(want.Items(), got.Items(), ctydebug.CmpOptions); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}
