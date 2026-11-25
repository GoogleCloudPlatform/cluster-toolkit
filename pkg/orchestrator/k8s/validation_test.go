// Copyright 2026 Google LLC
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

package k8s

import (
	"math"
	"strings"
	"testing"
)

func TestValidateNonNegativeInt32(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		flag    string
		in      int
		wantErr string
	}{
		{name: "zero allowed", flag: "--restarts", in: 0},
		{name: "positive allowed", flag: "--restarts", in: 5},
		{name: "int32 max allowed", flag: "--restarts", in: math.MaxInt32},
		{name: "negative rejected", flag: "--restarts", in: -1, wantErr: "--restarts must be non-negative"},
		{name: "custom flag name formatted", flag: "--pathways-max-slice-restarts", in: -1, wantErr: "--pathways-max-slice-restarts must be non-negative"},
	}
	if math.MaxInt > math.MaxInt32 {
		aboveInt32 := int64(math.MaxInt32) + 1
		cases = append(cases, struct {
			name    string
			flag    string
			in      int
			wantErr string
		}{name: "above int32 max rejected", flag: "--restarts", in: int(aboveInt32), wantErr: "--restarts must be at most 2147483647"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateNonNegativeInt32(tc.flag, tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ValidateNonNegativeInt32(%q, %d) err = %v, want substring %q", tc.flag, tc.in, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateNonNegativeInt32(%q, %d) unexpected err: %v", tc.flag, tc.in, err)
			}
		})
	}
}
