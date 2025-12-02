// Copyright 2025 Google LLC
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

package cmd

import (
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zclconf/go-cty/cty"
	compute "google.golang.org/api/compute/v1"
)

func TestGetNetworkNamesFromBlueprint(t *testing.T) {
	testCases := []struct {
		name           string
		deploymentName string
		group          config.Group
		want           map[string]bool
		wantErr        bool
	}{
		{
			name:           "vpc with custom name",
			deploymentName: "test-deployment",
			group: config.Group{
				Modules: []config.Module{
					{
						Source: "modules/network/vpc",
						Settings: config.NewDict(map[string]cty.Value{
							"network_name": cty.StringVal("custom-vpc-name"),
						}),
					},
				},
			},
			want:    map[string]bool{"custom-vpc-name": true},
			wantErr: false,
		},
		{
			name:           "vpc with default name",
			deploymentName: "test_deployment",
			group: config.Group{
				Modules: []config.Module{
					{
						Source:   "modules/network/vpc",
						Settings: config.NewDict(map[string]cty.Value{}),
					},
				},
			},
			want:    map[string]bool{"test-deployment-net": true},
			wantErr: false,
		},
		{
			name:           "multivpc",
			deploymentName: "test-deployment",
			group: config.Group{
				Modules: []config.Module{
					{
						Source: "modules/network/multivpc",
						Settings: config.NewDict(map[string]cty.Value{
							"network_name_prefix": cty.StringVal("multi-net"),
							"network_count":       cty.NumberIntVal(3),
						}),
					},
				},
			},
			want: map[string]bool{
				"multi-net-0": true,
				"multi-net-1": true,
				"multi-net-2": true,
			},
			wantErr: false,
		},
		{
			name:           "no network modules",
			deploymentName: "test-deployment",
			group: config.Group{
				Modules: []config.Module{
					{
						Source: "modules/compute/vm",
					},
				},
			},
			want:    map[string]bool{},
			wantErr: false,
		},
		{
			name:           "mixed modules",
			deploymentName: "test-deployment",
			group: config.Group{
				Modules: []config.Module{
					{
						Source: "modules/network/vpc",
						Settings: config.NewDict(map[string]cty.Value{
							"network_name": cty.StringVal("custom-vpc-name"),
						}),
					},
					{
						Source: "modules/network/multivpc",
						Settings: config.NewDict(map[string]cty.Value{
							"network_name_prefix": cty.StringVal("multi-net"),
							"network_count":       cty.NumberIntVal(2),
						}),
					},
				},
			},
			want: map[string]bool{
				"custom-vpc-name": true,
				"multi-net-0":     true,
				"multi-net-1":     true,
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := getNetworkNamesFromBlueprint(tc.deploymentName, tc.group)
			if (err != nil) != tc.wantErr {
				t.Errorf("getNetworkNamesFromBlueprint() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("getNetworkNamesFromBlueprint() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetStringVar(t *testing.T) {
	testCases := []struct {
		name    string
		vars    config.Dict
		key     string
		want    string
		wantErr bool
	}{
		{
			name:    "key exists and is string",
			vars:    config.NewDict(map[string]cty.Value{"test_key": cty.StringVal("test_value")}),
			key:     "test_key",
			want:    "test_value",
			wantErr: false,
		},
		{
			name:    "key does not exist",
			vars:    config.NewDict(map[string]cty.Value{"other_key": cty.StringVal("other_value")}),
			key:     "non_existent_key",
			want:    "",
			wantErr: true,
		},
		{
			name:    "key is null",
			vars:    config.NewDict(map[string]cty.Value{"null_key": cty.NilVal}),
			key:     "null_key",
			want:    "",
			wantErr: true,
		},
		{
			name:    "key is not a string",
			vars:    config.NewDict(map[string]cty.Value{"int_key": cty.NumberIntVal(123)}),
			key:     "int_key",
			want:    "",
			wantErr: true,
		},
		{
			name:    "key is an empty string",
			vars:    config.NewDict(map[string]cty.Value{"empty_key": cty.StringVal("")}),
			key:     "empty_key",
			want:    "",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := getStringVar(tc.vars, tc.key)
			if (err != nil) != tc.wantErr {
				t.Errorf("getStringVar() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if got != tc.want {
				t.Errorf("getStringVar() got = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetProjectAndDeploymentVars(t *testing.T) {
	testCases := []struct {
		name           string
		vars           config.Dict
		wantProjectID  string
		wantDeployment string
		wantErr        bool
	}{
		{
			name:           "both project_id and deployment_name exist",
			vars:           config.NewDict(map[string]cty.Value{"project_id": cty.StringVal("test-project"), "deployment_name": cty.StringVal("test-deployment")}),
			wantProjectID:  "test-project",
			wantDeployment: "test-deployment",
			wantErr:        false,
		},
		{
			name:           "project_id missing",
			vars:           config.NewDict(map[string]cty.Value{"deployment_name": cty.StringVal("test-deployment")}),
			wantProjectID:  "",
			wantDeployment: "",
			wantErr:        true,
		},
		{
			name:           "deployment_name missing",
			vars:           config.NewDict(map[string]cty.Value{"project_id": cty.StringVal("test-project")}),
			wantProjectID:  "",
			wantDeployment: "",
			wantErr:        true,
		},
		{
			name:           "project_id not a string",
			vars:           config.NewDict(map[string]cty.Value{"project_id": cty.NumberIntVal(123), "deployment_name": cty.StringVal("test-deployment")}),
			wantProjectID:  "",
			wantDeployment: "",
			wantErr:        true,
		},
		{
			name:           "deployment_name not a string",
			vars:           config.NewDict(map[string]cty.Value{"project_id": cty.StringVal("test-project"), "deployment_name": cty.NumberIntVal(123)}),
			wantProjectID:  "",
			wantDeployment: "",
			wantErr:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotProjectID, gotDeployment, err := getProjectAndDeploymentVars(tc.vars)
			if (err != nil) != tc.wantErr {
				t.Errorf("getProjectAndDeploymentVars() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if gotProjectID != tc.wantProjectID {
				t.Errorf("getProjectAndDeploymentVars() gotProjectID = %v, want %v", gotProjectID, tc.wantProjectID)
			}
			if gotDeployment != tc.wantDeployment {
				t.Errorf("getProjectAndDeploymentVars() gotDeployment = %v, want %v", gotDeployment, tc.wantDeployment)
			}
		})
	}
}

func TestDestroyGroupsUnsupportedKind(t *testing.T) {
	// Mock dependencies if necessary
	// For this test, we primarily care about the group.Kind() switch.

	bp := config.Blueprint{
		Groups: []config.Group{
			{
				Name:    "unsupported-group",
				Modules: []config.Module{},
			},
		},
	}
	ctx := &config.YamlCtx{}

	destroyFailed, _ := destroyGroups("", "", bp, ctx)

	if !destroyFailed {
		t.Errorf("destroyGroups() failed to report destruction failure for unsupported kind.")
	}
}

type mockFirewallDeleter struct {
	deleteErr error
}

func (m *mockFirewallDeleter) FirewallsDelete(projectID string, firewall string) (*compute.Operation, error) {
	return nil, m.deleteErr
}

func TestConfirmAndDeleteFirewallRulesWithError(t *testing.T) {
	// Bypass the interactive prompt for this test
	originalFlagAutoApprove := flagAutoApprove
	flagAutoApprove = true
	defer func() { flagAutoApprove = originalFlagAutoApprove }()

	// Create a mock compute service that returns an error on delete
	mockService := &mockFirewallDeleter{
		deleteErr: fmt.Errorf("mock delete error"),
	}

	firewallsToDelete := []*compute.Firewall{
		{Name: "fw-1"},
		{Name: "fw-2"},
	}

	err := confirmAndDeleteFirewallRules("test-project", "test-deployment", mockService, firewallsToDelete)

	if err == nil {
		t.Errorf("confirmAndDeleteFirewallRules() did not return an error when it should have.")
	}

	// In a real-world scenario, you would also capture and check the log output.
	// For this test, we are primarily concerned with the error propagation.
}

func TestRunDestroyCmd(t *testing.T) {
	testCases := []struct {
		name          string
		robust        bool
		autoApprove   bool
		destroyFailed bool
		expectFatal   bool
	}{
		{
			name:          "robust and auto-approve",
			robust:        true,
			autoApprove:   true,
			destroyFailed: false,
			expectFatal:   false,
		},
		{
			name:          "robust and no auto-approve",
			robust:        true,
			autoApprove:   false,
			destroyFailed: false,
			expectFatal:   false,
		},
		{
			name:          "no robust and auto-approve",
			robust:        false,
			autoApprove:   true,
			destroyFailed: false,
			expectFatal:   false,
		},
		{
			name:          "no robust and no auto-approve",
			robust:        false,
			autoApprove:   false,
			destroyFailed: false,
			expectFatal:   false,
		},
		{
			name:          "robust and destroy failed",
			robust:        true,
			autoApprove:   true,
			destroyFailed: true,
			expectFatal:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Mock destroyGroups to control its behavior
			destroyGroupsFunc = func(deplRoot string, artifactsDir string, bp config.Blueprint, ctx *config.YamlCtx) (bool, []string) {
				return tc.destroyFailed, []string{}
			}

			// Set the flags
			robustDestroy = tc.robust
			flagAutoApprove = tc.autoApprove

			// Create a dummy deployment directory
			deplRoot, err := os.MkdirTemp("", "test-depl")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(deplRoot)

			// Create a dummy artifacts directory
			artifactsDir := filepath.Join(deplRoot, ".ghpc", "artifacts")
			if err := os.MkdirAll(artifactsDir, 0755); err != nil {
				t.Fatalf("Failed to create artifacts dir: %v", err)
			}

			// Create a dummy blueprint file
			bpFile := filepath.Join(artifactsDir, "expanded_blueprint.yaml")
			if err := os.WriteFile(bpFile, []byte(""), 0644); err != nil {
				t.Fatalf("Failed to write dummy blueprint file: %v", err)
			}

			// Capture logging output
			_, w, _ := os.Pipe()
			originalStderr := os.Stderr
			os.Stderr = w
			defer func() {
				os.Stderr = originalStderr
			}()

			// Use a deferred function to check for fatal errors
			defer func() {
				if r := recover(); r != nil {
					if !tc.expectFatal {
						t.Errorf("runDestroyCmd() panicked unexpectedly: %v", r)
					}
				} else {
					if tc.expectFatal {
						t.Errorf("runDestroyCmd() did not panic when it should have.")
					}
				}
			}()

			// Override the exit function to prevent the test from exiting
			originalExit := logging.Exit
			defer func() { logging.Exit = originalExit }()
			logging.Exit = func(code int) {
				panic(fmt.Sprintf("exit %d", code))
			}

			destroyRunner(deplRoot, artifactsDir, config.Blueprint{}, &config.YamlCtx{})
		})
	}
}
