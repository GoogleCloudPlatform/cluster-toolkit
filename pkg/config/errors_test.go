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
	"errors"
	"testing"
)

func TestPosErr(t *testing.T) {
	err := errors.New("mango")
	want := "line 4, col 31: mango"
	got := PosError{Pos: Pos{Line: 4, Col: 31}, Err: err}

	if got.Error() != want {
		t.Errorf("got %q, want %q", got.Error(), want)
	}
	if !errors.Is(got, err) {
		t.Errorf("got %#v, want %#v", errors.Unwrap(got), err)
	}

}
