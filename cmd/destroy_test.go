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
	"testing"

	"github.com/zclconf/go-cty/cty"
	compute "google.golang.org/api/compute/v1"
)

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
			destroyGroupsFunc = func(_ string, _ string, _ config.Blueprint, _ *config.YamlCtx) (bool, []string) {
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

func TestGroupHasNetworkModule(t *testing.T) {
	testCases := []struct {
		name  string
		group config.Group
		want  bool
	}{
		{
			name: "official network module",
			group: config.Group{
				Modules: []config.Module{{Source: "modules/network/vpc"}},
			},
			want: true,
		},
		{
			name: "community network module",
			group: config.Group{
				Modules: []config.Module{{Source: "community/modules/network/vpc"}},
			},
			want: true,
		},
		{
			name: "no network module",
			group: config.Group{
				Modules: []config.Module{{Source: "modules/compute/vm"}},
			},
			want: false,
		},
		{
			name: "unrelated module",
			group: config.Group{
				Modules: []config.Module{{Source: "modules/other/module"}},
			},
			want: false,
		},
		{
			name:  "empty group",
			group: config.Group{},
			want:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := groupHasNetworkModule(tc.group); got != tc.want {
				t.Errorf("groupHasNetworkModule() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDestroyGroupsRobust(t *testing.T) {
	// a group that will trigger the cleanup logic
	networkGroup := config.Group{
		Name: "network-group",
		Modules: []config.Module{
			{Source: "modules/network/vpc", Kind: config.TerraformKind},
		},
	}

	// a group that will not
	computeGroup := config.Group{
		Name: "compute-group",
		Modules: []config.Module{
			{Source: "modules/compute/vm-instance", Kind: config.TerraformKind},
		},
	}

	// blueprint with valid vars for cleanup
	bpWithVars := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"project_id":      cty.StringVal("test-project"),
			"deployment_name": cty.StringVal("test-deployment"),
		}),
	}

	// blueprint with missing vars
	bpWithoutVars := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{}),
	}

	// Restore original state after tests
	originalRobustDestroy := robustDestroy
	originalCleanupFunc := cleanupFirewallRulesFunc
	originalDestroyTerraformGroupFunc := destroyTerraformGroupFunc
	defer func() {
		robustDestroy = originalRobustDestroy
		cleanupFirewallRulesFunc = originalCleanupFunc
		destroyTerraformGroupFunc = originalDestroyTerraformGroupFunc
	}()

	testCases := []struct {
		name              string
		bp                config.Blueprint
		group             config.Group
		robust            bool
		cleanupErr        error // The error for the mocked cleanupFunc to return
		expectCleanupCall bool
		expectFail        bool
	}{
		{
			name:              "robust flag off",
			bp:                bpWithVars,
			group:             networkGroup,
			robust:            false,
			cleanupErr:        nil,
			expectCleanupCall: false,
			expectFail:        false,
		},
		{
			name:              "not a network group",
			bp:                bpWithVars,
			group:             computeGroup,
			robust:            true,
			cleanupErr:        nil,
			expectCleanupCall: false,
			expectFail:        false,
		},
		{
			name:              "get vars fails",
			bp:                bpWithoutVars,
			group:             networkGroup,
			robust:            true,
			cleanupErr:        nil,
			expectCleanupCall: false,
			expectFail:        true,
		},
		{
			name:              "cleanup fails",
			bp:                bpWithVars,
			group:             networkGroup,
			robust:            true,
			cleanupErr:        fmt.Errorf("mock cleanup error"),
			expectCleanupCall: true,
			expectFail:        true,
		},
		{
			name:              "cleanup succeeds",
			bp:                bpWithVars,
			group:             networkGroup,
			robust:            true,
			cleanupErr:        nil,
			expectCleanupCall: true,
			expectFail:        false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			robustDestroy = tc.robust
			cleanupCalled := false
			cleanupFirewallRulesFunc = func(projectID, deploymentName string) error {
				cleanupCalled = true
				if projectID != "test-project" || deploymentName != "test-deployment" {
					t.Errorf("cleanup function called with wrong args: got %s, %s", projectID, deploymentName)
				}
				return tc.cleanupErr
			}

			destroyTerraformGroupFunc = func(groupDir string) error {
				return nil // Do nothing
			}

			// The blueprint for the test run
			bp := tc.bp
			bp.Groups = []config.Group{tc.group}

			// We only care about the failure flag, not the packer manifests
			destroyFailed, _ := destroyGroups("", "", bp, &config.YamlCtx{})

			if cleanupCalled != tc.expectCleanupCall {
				t.Errorf("cleanupFirewallRules call was %v, expected %v", cleanupCalled, tc.expectCleanupCall)
			}

			if destroyFailed != tc.expectFail {
				t.Errorf("destroyGroups failed = %v, want %v", destroyFailed, tc.expectFail)
			}
		})
	}
}

func TestFilterString(t *testing.T) {
	deploymentName := "test-deployment"
	expectedFilter := `name eq ".*test-deployment.*"`
	actualFilter := fmt.Sprintf(`name eq ".*%s.*"`, deploymentName)

	if actualFilter != expectedFilter {
		t.Errorf("Filter string mismatch: got %q, want %q", actualFilter, expectedFilter)
	}
}
