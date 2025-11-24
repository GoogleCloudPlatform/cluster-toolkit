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

package validators

import (
	"hpc-toolkit/pkg/config"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestCheckSlurmProvisioning(t *testing.T) {
	testCases := []struct {
		name        string
		vars        map[string]cty.Value
		expectError bool
	}{
		{
			name: "no provisioning vars",
			vars: map[string]cty.Value{
				"other_var": cty.StringVal("value"),
			},
			expectError: false,
		},
		{
			name: "reservation set",
			vars: map[string]cty.Value{
				"reservation_name": cty.StringVal("my-reservation"),
			},
			expectError: false,
		},
		{
			name: "spot enabled",
			vars: map[string]cty.Value{
				"enable_spot_vm": cty.BoolVal(true),
			},
			expectError: false,
		},
		{
			name: "dws flex enabled",
			vars: map[string]cty.Value{
				"dws_flex_enabled": cty.BoolVal(true),
			},
			expectError: false,
		},
		{
			name: "no provisioning selected",
			vars: map[string]cty.Value{
				"reservation_name": cty.StringVal(""),
				"enable_spot_vm":   cty.BoolVal(false),
			},
			expectError: true,
		},
		{
			name: "multiple provisioning selected",
			vars: map[string]cty.Value{
				"reservation_name": cty.StringVal("my-reservation"),
				"enable_spot_vm":   cty.BoolVal(true),
			},
			expectError: true,
		},
		{
			name: "prefixed reservation set",
			vars: map[string]cty.Value{
				"prefix_reservation_name": cty.StringVal("my-reservation"),
			},
			expectError: false,
		},
		{
			name: "prefixed spot enabled",
			vars: map[string]cty.Value{
				"prefix_enable_spot_vm": cty.BoolVal(true),
			},
			expectError: false,
		},
		{
			name: "prefixed dws flex enabled",
			vars: map[string]cty.Value{
				"prefix_dws_flex_enabled": cty.BoolVal(true),
			},
			expectError: false,
		},
		{
			name: "prefixed no provisioning selected",
			vars: map[string]cty.Value{
				"prefix_reservation_name": cty.StringVal(""),
				"prefix_enable_spot_vm":   cty.BoolVal(false),
			},
			expectError: true,
		},
		{
			name: "prefixed multiple provisioning selected",
			vars: map[string]cty.Value{
				"prefix_reservation_name": cty.StringVal("my-reservation"),
				"prefix_enable_spot_vm":   cty.BoolVal(true),
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bp := config.Blueprint{
				Vars: config.NewDict(tc.vars),
			}
			err := checkSlurmProvisioning(bp)
			if (err != nil) != tc.expectError {
				t.Errorf("checkSlurmProvisioning() with vars = %v; got error = %v, want error = %v", tc.vars, err, tc.expectError)
			}
		})
	}
}

func TestCheckSlurmNodeCount(t *testing.T) {
	testCases := []struct {
		name        string
		vars        map[string]cty.Value
		groups      []config.Group
		expectError bool
	}{
		{
			name: "static count > 0",
			groups: []config.Group{
				{
					Name: "test-group",
					Modules: []config.Module{
						{
							ID:     "test-nodeset",
							Source: "nodeset",
							Settings: config.NewDict(map[string]cty.Value{
								"node_count_static": cty.NumberIntVal(1),
							}),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "dynamic count > 0",
			groups: []config.Group{
				{
					Name: "test-group",
					Modules: []config.Module{
						{
							ID:     "test-nodeset",
							Source: "nodeset",
							Settings: config.NewDict(map[string]cty.Value{
								"node_count_dynamic_max": cty.NumberIntVal(1),
							}),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "both counts zero",
			groups: []config.Group{
				{
					Name: "test-group",
					Modules: []config.Module{
						{
							ID:     "test-nodeset",
							Source: "nodeset",
							Settings: config.NewDict(map[string]cty.Value{
								"node_count_static":      cty.NumberIntVal(0),
								"node_count_dynamic_max": cty.NumberIntVal(0),
							}),
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "static count from var",
			vars: map[string]cty.Value{
				"my_count": cty.NumberIntVal(2),
			},
			groups: []config.Group{
				{
					Name: "test-group",
					Modules: []config.Module{
						{
							ID:     "test-nodeset",
							Source: "nodeset",
							Settings: config.NewDict(map[string]cty.Value{
								"node_count_static": cty.StringVal("$(vars.my_count)"),
							}),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "static count from var zero",
			vars: map[string]cty.Value{
				"my_count": cty.NumberIntVal(0),
			},
			groups: []config.Group{
				{
					Name: "test-group",
					Modules: []config.Module{
						{
							ID:     "test-nodeset",
							Source: "nodeset",
							Settings: config.NewDict(map[string]cty.Value{
								"node_count_static":      cty.StringVal("$(vars.my_count)"),
								"node_count_dynamic_max": cty.NumberIntVal(0),
							}),
						},
					},
				},
			},
			expectError: true,
		},
		{
			name:        "no nodeset modules",
			groups:      []config.Group{},
			expectError: false,
		},
		{
			name: "static count as string number",
			groups: []config.Group{
				{
					Name: "test-group",
					Modules: []config.Module{
						{
							ID:     "test-nodeset",
							Source: "nodeset",
							Settings: config.NewDict(map[string]cty.Value{
								"node_count_static": cty.StringVal("2"),
							}),
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bp := config.Blueprint{
				Vars:   config.NewDict(tc.vars),
				Groups: tc.groups,
			}
			err := checkSlurmNodeCount(bp)
			if (err != nil) != tc.expectError {
				t.Errorf("checkSlurmNodeCount() with groups = %v; got error = %v, want error = %v", tc.groups, err, tc.expectError)
			}
		})
	}
}

func TestCheckSlurmClusterName(t *testing.T) {
	testCases := []struct {
		name        string
		vars        map[string]cty.Value
		groups      []config.Group
		expectError bool
	}{
		{
			name: "valid name at blueprint level",
			vars: map[string]cty.Value{
				"slurm_cluster_name": cty.StringVal("validname"),
			},
			expectError: false,
		},
		{
			name: "invalid name too long",
			vars: map[string]cty.Value{
				"slurm_cluster_name": cty.StringVal("invalidnametoolong"),
			},
			expectError: true,
		},
		{
			name: "invalid name with hyphen",
			vars: map[string]cty.Value{
				"slurm_cluster_name": cty.StringVal("with-hy"),
			},
			expectError: true,
		},
		{
			name: "valid name at module level",
			groups: []config.Group{
				{
					Name: "test-group",
					Modules: []config.Module{
						{
							ID: "test-controller",
							Settings: config.NewDict(map[string]cty.Value{
								"slurm_cluster_name": cty.StringVal("validname"),
							}),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid name at module level",
			groups: []config.Group{
				{
					Name: "test-group",
					Modules: []config.Module{
						{
							ID: "test-controller",
							Settings: config.NewDict(map[string]cty.Value{
								"slurm_cluster_name": cty.StringVal("invalid-name"),
							}),
						},
					},
				},
			},
			expectError: true,
		},
		{
			name:        "no name defined",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bp := config.Blueprint{
				Vars:   config.NewDict(tc.vars),
				Groups: tc.groups,
			}
			err := checkSlurmClusterName(bp)
			if (err != nil) != tc.expectError {
				t.Errorf("checkSlurmClusterName() with vars = %v and groups = %v; got error = %v, want error = %v", tc.vars, tc.groups, err, tc.expectError)
			}
		})
	}
}