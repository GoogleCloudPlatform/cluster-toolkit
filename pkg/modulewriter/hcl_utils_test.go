// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modulewriter

import (
	"hpc-toolkit/pkg/modulereader"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/zclconf/go-cty/cty"
)

func TestHclAtttributesRW(t *testing.T) {
	want := make(map[string]cty.Value)
	// test that a string that needs escaping when written is read correctly
	want["key1"] = cty.StringVal("${value1}")

	fn, err := os.CreateTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fn.Name())

	err = WriteHclAttributes(want, fn.Name())
	if err != nil {
		t.Errorf("could not write HCL attributes file")
	}

	got, err := modulereader.ReadHclAttributes(fn.Name())
	if err != nil {
		t.Errorf("could not read HCL attributes file")
	}

	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(cty.Value{})); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}
