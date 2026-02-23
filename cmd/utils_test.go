// Copyright 2026 Google LLC
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
	"fmt"
	"hpc-toolkit/pkg/config"
	"testing"
)

func TestIsGroupSelected(t *testing.T) {
	type test struct {
		only  []string
		skip  []string
		group config.GroupName
		want  bool
	}
	tests := []test{
		{nil, nil, "green", true},
		{[]string{"green"}, nil, "green", true},
		{[]string{"green"}, nil, "blue", false},
		{nil, []string{"green"}, "green", false},
		{nil, []string{"green"}, "blue", true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%v;%v;%q", tc.only, tc.skip, tc.group), func(t *testing.T) {
			flagOnlyGroups, flagSkipGroups = tc.only, tc.skip
			got := isGroupSelected(tc.group)
			if got != tc.want {
				t.Errorf("isGroupSelected(%v) = %v; want %v", tc.group, got, tc.want)
			}
		})
	}
}

func TestValidateGroupSelectionFlags(t *testing.T) {
	type test struct {
		only   []string
		skip   []string
		groups []string
		err    bool
	}
	tests := []test{
		{nil, nil, []string{"green"}, false},
		{[]string{"green"}, []string{"blue"}, []string{"green", "blue"}, true},
		{[]string{"green"}, nil, []string{"green"}, false},
		{[]string{"green"}, nil, []string{"blue"}, true},
		{nil, []string{"green"}, []string{"green"}, false},
		{nil, []string{"green"}, []string{"blue"}, true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%v;%v;%v", tc.only, tc.skip, tc.groups), func(t *testing.T) {
			flagOnlyGroups, flagSkipGroups = tc.only, tc.skip
			bp := config.Blueprint{}
			for _, g := range tc.groups {
				bp.Groups = append(bp.Groups, config.Group{Name: config.GroupName(g)})
			}

			err := validateGroupSelectionFlags(bp)
			if tc.err && err == nil {
				t.Errorf("validateGroupSelectionFlags(%v) = nil; want error", tc.groups)
			}
			if !tc.err && err != nil {
				t.Errorf("validateGroupSelectionFlags(%v) = %v; want nil", tc.groups, err)
			}
		})
	}

}
