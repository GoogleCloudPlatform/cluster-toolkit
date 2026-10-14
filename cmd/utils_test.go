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

	"github.com/spf13/cobra"
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

func TestAddParallelismFlag(t *testing.T) {
	origFlag := flagParallelism
	defer func() { flagParallelism = origFlag }()

	testCmd := &cobra.Command{Use: "test"}
	retCmd := addParallelismFlag(testCmd)
	if retCmd != testCmd {
		t.Errorf("addParallelismFlag should return the passed command")
	}

	flag := testCmd.Flags().Lookup("parallelism")
	if flag == nil {
		t.Fatalf("expected --parallelism flag to be registered on cmd")
	}
	if flag.DefValue != "0" {
		t.Errorf("expected default value '0', got %q", flag.DefValue)
	}

	expectedUsage := "Limit the number of concurrent operations in Terraform (default: 10, or GCLUSTER_TERRAFORM_PARALLELISM)"
	if flag.Usage != expectedUsage {
		t.Errorf("expected usage %q, got %q", expectedUsage, flag.Usage)
	}

	flagParallelism = 0
	if err := testCmd.ParseFlags([]string{"--parallelism", "80"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}
	if flagParallelism != 80 {
		t.Errorf("expected flagParallelism to be 80, got %d", flagParallelism)
	}
}

func TestValidateParallelismFlag(t *testing.T) {
	origFlag := flagParallelism
	defer func() { flagParallelism = origFlag }()

	testCases := []struct {
		val       int
		shouldErr bool
	}{
		{-100, true},
		{-10, true},
		{-1, true},
		{0, false},
		{1, false},
		{10, false},
		{80, false},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("parallelism_%d", tc.val), func(t *testing.T) {
			flagParallelism = tc.val
			err := validateParallelismFlag()
			if tc.shouldErr && err == nil {
				t.Errorf("validateParallelismFlag() with %d expected error, got nil", tc.val)
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("validateParallelismFlag() with %d expected nil, got error: %v", tc.val, err)
			}
		})
	}
}

func TestPreRunEParallelismValidation(t *testing.T) {
	origFlag := flagParallelism
	defer func() { flagParallelism = origFlag }()

	cmds := []*cobra.Command{deployCmd, destroyCmd, exportCmd}
	for _, c := range cmds {
		t.Run(c.Name(), func(t *testing.T) {
			if c.PreRunE == nil {
				t.Fatalf("command %s has nil PreRunE", c.Name())
			}
			flagParallelism = -5
			if err := c.PreRunE(c, []string{}); err == nil {
				t.Errorf("command %s PreRunE expected error for negative parallelism (-5), got nil", c.Name())
			}
			flagParallelism = 0
			if err := c.PreRunE(c, []string{}); err != nil {
				t.Errorf("command %s PreRunE expected nil for parallelism 0, got %v", c.Name(), err)
			}
			flagParallelism = 40
			if err := c.PreRunE(c, []string{}); err != nil {
				t.Errorf("command %s PreRunE expected nil for parallelism 40, got %v", c.Name(), err)
			}
		})
	}
}
