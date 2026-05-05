// Copyright 2026 "Google LLC"
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

package telemetry

import (
	"context"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"os"
	"path/filepath"
	"testing"
	"time"

	"runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zclconf/go-cty/cty"
)

func TestNewCollector(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	// Passing nil for args prevents getBlueprint from attempting to read a file
	c := NewCollector(cmd, nil, SOURCE)

	if c == nil {
		t.Fatal("Expected NewCollector to return a valid Collector, got nil")
	}
	if c.eventCmd != cmd {
		t.Errorf("Expected eventCmd to be %v, got %v", cmd, c.eventCmd)
	}
	if c.metadata == nil {
		t.Error("Expected metadata map to be initialized")
	}
}

// TestCollectMetrics_Extensible uses a table-driven approach.
// Future metrics can be seamlessly verified by adding keys to `expectedKeys`
// and values to `expectedValues`.
func TestCollectMetrics_Extensible(t *testing.T) {
	// Define all expected metric keys from types.go
	expectedKeys := []string{
		COMMAND_FLAGS,
		MACHINE_TYPE,
		REGION,
		ZONE,
		OS_NAME,
		OS_VERSION,
		TERRAFORM_VERSION,
		BILLING_ACCOUNT_ID,
		INSTALLATION_MODE,
		IS_TEST_DATA,
		EXIT_CODE,
	}

	tests := []struct {
		name             string
		errorCode        int
		installationMode string
		setupCmd         func(cmd *cobra.Command) // Hook to configure the command
		setupCollector   func(c *Collector)       // Hook to mock internal collector state
		expectedValues   map[string]string
	}{
		{
			name:             "Success exit code",
			errorCode:        0,
			installationMode: SOURCE,
			setupCmd: func(cmd *cobra.Command) {
				// Define dummy flags for the mock command
				cmd.Flags().Bool("force", false, "Force execution")
				cmd.Flags().String("project", "", "GCP Project")

				// Simulate the user providing these flags at runtime
				_ = cmd.Flags().Set("force", "true")
				_ = cmd.Flags().Set("project", "test-project")
			},
			setupCollector: func(c *Collector) {
				// Mock the blueprint variables and modules
				c.blueprint = config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{
						"region": cty.StringVal("us-central1"),
						"zone":   cty.StringVal("us-central1-a"),
					}),
					Groups: []config.Group{
						{
							Name: config.GroupName("primary"),
							Modules: []config.Module{
								{
									ID: config.ModuleID("compute_pool"),
									// Ensure the source matches the machineTypeModulePattern ".*modules.compute.*"
									Source: "modules/compute/vm-instance",
									Settings: config.NewDict(map[string]cty.Value{
										"machine_type": cty.StringVal("c2-standard-8"),
									}),
								},
							},
						},
					},
				}
			},
			expectedValues: map[string]string{
				IS_TEST_DATA:       "true",
				EXIT_CODE:          "0",
				COMMAND_FLAGS:      "force,project",
				REGION:             "us-central1",
				ZONE:               "us-central1-a",
				MACHINE_TYPE:       "c2-standard-8",
				OS_NAME:            getOSName(),           // Dynamically expect the current OS name
				OS_VERSION:         getOSVersion(),        // Dynamically expect the current OS version
				TERRAFORM_VERSION:  getTerraformVersion(), // Dynamically expect the current Terraform version
				BILLING_ACCOUNT_ID: "",
				INSTALLATION_MODE:  SOURCE,
			},
		},
		{
			name:             "Failure exit code with missing region, zone, and machine type",
			errorCode:        1,
			installationMode: BINARY,
			setupCmd: func(cmd *cobra.Command) {
				// No flags set
			},
			setupCollector: func(c *Collector) {
				// Blueprint with empty vars
				c.blueprint = config.Blueprint{
					Vars:   config.NewDict(map[string]cty.Value{}),
					Groups: []config.Group{},
				}
			},
			expectedValues: map[string]string{
				IS_TEST_DATA:       "true",
				EXIT_CODE:          "1",
				COMMAND_FLAGS:      "",
				REGION:             "",
				ZONE:               "",
				OS_NAME:            getOSName(),           // Verify OS info is still collected on failure
				OS_VERSION:         getOSVersion(),        // Verify OS info is still collected on failure
				TERRAFORM_VERSION:  getTerraformVersion(), // Verify Terraform version is still collected on failure
				MACHINE_TYPE:       "",                    // Verify empty machine type when no matching modules exist
				BILLING_ACCOUNT_ID: "",
				INSTALLATION_MODE:  BINARY,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "mock"}

			// Execute the setup function to apply flags to the command
			if tt.setupCmd != nil {
				tt.setupCmd(cmd)
			}

			// Initialize the collector
			c := NewCollector(cmd, []string{}, tt.installationMode)

			// Execute the setup function to apply the blueprint state to the collector
			if tt.setupCollector != nil {
				tt.setupCollector(c)
			}

			// Run the method being tested
			c.CollectMetrics(tt.errorCode)

			// Assert that all expected keys are populated in the metadata
			for _, key := range expectedKeys {
				if _, exists := c.metadata[key]; !exists {
					t.Errorf("Expected key %q missing from metadata", key)
				}
			}

			// Assert that the specifically expected values match
			for key, expectedVal := range tt.expectedValues {
				if actualVal, exists := c.metadata[key]; !exists || actualVal != expectedVal {
					t.Errorf("For key %q, expected value %q, got %q", key, expectedVal, actualVal)
				}
			}
		})
	}
}

func TestBuildConcordEvent(t *testing.T) {
	rootCmd := &cobra.Command{Use: "gcluster"}
	childCmd := &cobra.Command{Use: "deploy"}
	rootCmd.AddCommand(childCmd)

	c := NewCollector(childCmd, nil, SOURCE)
	c.CollectMetrics(0)

	event := c.BuildConcordEvent()

	if event.ConsoleType != CLUSTER_TOOLKIT {
		t.Errorf("BuildConcordEvent() ConsoleType = %v, want %v", event.ConsoleType, CLUSTER_TOOLKIT)
	}
	if event.EventType != "gclusterCLI" {
		t.Errorf("BuildConcordEvent() EventType = %v, want gclusterCLI", event.EventType)
	}
	if event.EventName != "deploy" {
		t.Errorf("BuildConcordEvent() EventName = %v, want deploy", event.EventName)
	}
	if event.LatencyMs < 0 {
		t.Errorf("BuildConcordEvent() LatencyMs = %v, want >= 0", event.LatencyMs)
	}
	if event.ReleaseVersion == "" {
		t.Error("BuildConcordEvent() ReleaseVersion is empty")
	}

	// Verify metadata KV pairs mapping
	foundExitCode := false
	for _, meta := range event.EventMetadata {
		if meta["key"] == EXIT_CODE && meta["value"] == "0" {
			foundExitCode = true
			break
		}
	}

	if !foundExitCode {
		t.Errorf("BuildConcordEvent() EventMetadata did not properly translate metadata key-value pairs")
	}
}

func TestGetCommandName(t *testing.T) {
	tests := []struct {
		name     string
		cmdSetup func() *cobra.Command
		want     string
	}{
		{
			name: "Empty path",
			cmdSetup: func() *cobra.Command {
				return &cobra.Command{}
			},
			want: "",
		},
		{
			name: "Root command",
			cmdSetup: func() *cobra.Command {
				return &cobra.Command{Use: "gcluster"}
			},
			want: "gcluster",
		},
		{
			name: "Subcommand",
			cmdSetup: func() *cobra.Command {
				root := &cobra.Command{Use: "gcluster"}
				sub := &cobra.Command{Use: "job"}
				root.AddCommand(sub)
				return sub
			},
			want: "job",
		},
		{
			name: "Nested subcommand",
			cmdSetup: func() *cobra.Command {
				root := &cobra.Command{Use: "gcluster"}
				sub := &cobra.Command{Use: "job"}
				subsub := &cobra.Command{Use: "cancel"}
				root.AddCommand(sub)
				sub.AddCommand(subsub)
				return subsub
			},
			want: "job cancel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmdSetup()
			if got := getCommandName(cmd); got != tt.want {
				t.Errorf("getCommandName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetReleaseVersion(t *testing.T) {
	if got := getReleaseVersion(); got == "" {
		t.Errorf("getReleaseVersion() returned an empty string, expected toolkit version")
	}
}

func TestGetIsTestData(t *testing.T) {
	if got := getIsTestData(); got != "true" {
		t.Errorf("getIsTestData() = %v, want true", got)
	}
}

func TestGetLatencyMs(t *testing.T) {
	// Set the start time to 50ms in the past to test positive latency
	eventStartTime := time.Now().Add(-50 * time.Millisecond)
	latency := getLatencyMs(eventStartTime)

	if latency < 50 {
		t.Errorf("getLatencyMs() expected >= 50ms, got %d ms", latency)
	}
}

func TestGetClientInstallId(t *testing.T) {
	// Ensure Viper is reset after all tests to prevent config leakage
	defer viper.Reset()

	tests := []struct {
		name       string
		mockConfig func()
		want       string
	}{
		{
			name: "returns valid client install id when set in config",
			mockConfig: func() {
				// Mocks the case where the CLI has already bootstrapped the config
				// and saved a hashed persistent user_id.
				viper.Set("user_id", "a1b2c3d4e5f6")
			},
			want: "a1b2c3d4e5f6",
		},
		{
			name: "returns empty string when client install id is missing",
			mockConfig: func() {
				// Mocks the case where the config is missing or uninitialized.
				viper.Set("user_id", "")
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset Viper state before each sub-test to ensure isolation
			viper.Reset()

			// Apply the mocked configuration for this specific test case
			tt.mockConfig()

			// Act
			got := getClientInstallId()

			// Assert
			if got != tt.want {
				t.Errorf("getClientInstallId() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetCmdFlags(t *testing.T) {
	tests := []struct {
		name     string
		setupCmd func(cmd *cobra.Command)
		expected string
	}{
		{
			name: "No flags set",
			setupCmd: func(cmd *cobra.Command) {
				// Define a flag but do not set it
				cmd.Flags().Bool("force", false, "Force execution")
			},
			expected: "",
		},
		{
			name: "Single flag set",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().Bool("force", false, "Force execution")

				// Simulate user passing --force
				_ = cmd.Flags().Set("force", "true")
			},
			expected: "force",
		},
		{
			name: "Multiple flags set",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().Bool("force", false, "Force execution")
				cmd.Flags().String("project", "", "GCP Project")
				cmd.Flags().Int("retries", 3, "Number of retries")

				// Simulate user passing --force and --project
				_ = cmd.Flags().Set("force", "true")
				_ = cmd.Flags().Set("project", "test-project")
				// Leave "retries" unset to ensure it isn't collected
			},
			// pflag typically stores flags in alphabetical order, but adjust if your function sorts them differently
			expected: "force,project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "mock"}

			if tt.setupCmd != nil {
				tt.setupCmd(cmd)
			}

			actual := getCmdFlags(cmd)

			if actual != tt.expected {
				t.Errorf("getCmdFlags() = %q, want %q", actual, tt.expected)
			}
		})
	}
}

// TestGetKeyFromBlueprint verifies that the keys are correctly extracted from the blueprint.
func TestGetKeyFromBlueprint(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		setupBp  func() config.Blueprint
		expected string
	}{
		{
			name: "Valid region",
			key:  "region",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{
						"region": cty.StringVal("us-central1"),
					}),
				}
			},
			expected: "us-central1",
		},
		{
			name: "Valid zone",
			key:  "zone",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{
						"zone": cty.StringVal("us-central1-a"),
					}),
				}
			},
			expected: "us-central1-a",
		},
		{
			name: "Missing key",
			key:  "zone",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{}),
				}
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := tt.setupBp()
			actual := getKeyFromBlueprint(tt.key, bp)

			if actual != tt.expected {
				t.Errorf("getKeyFromBlueprint(%q) = %q, want %q", tt.key, actual, tt.expected)
			}
		})
	}
}

// TestGetMachineType verifies that machine types are correctly extracted from the blueprint.
func TestGetMachineType(t *testing.T) {
	tests := []struct {
		name     string
		setupBp  func() config.Blueprint
		expected string
	}{
		{
			name: "Single machine type in module",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Groups: []config.Group{
						{
							Name: config.GroupName("primary"),
							Modules: []config.Module{
								{
									ID:     config.ModuleID("compute_pool"),
									Source: "modules/compute/vm-instance",
									Settings: config.NewDict(map[string]cty.Value{
										"machine_type": cty.StringVal("c2-standard-8"),
									}),
								},
							},
						},
					},
				}
			},
			expected: "c2-standard-8",
		},
		{
			name: "Multiple different machine types",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Groups: []config.Group{
						{
							Name: config.GroupName("primary"),
							Modules: []config.Module{
								{
									ID:     config.ModuleID("login_node"),
									Source: "modules/compute/vm-instance",
									Settings: config.NewDict(map[string]cty.Value{
										"machine_type": cty.StringVal("n2-standard-2"),
									}),
								},
								{
									ID:     config.ModuleID("lcontroller_node"),
									Source: "modules/compute/vm-instance",
									Settings: config.NewDict(map[string]cty.Value{
										"machine_type": cty.StringVal("n2-standard-2"),
									}),
								},
								{
									ID:     config.ModuleID("compute_pool"),
									Source: "modules/compute/gke-node-pool",
									Settings: config.NewDict(map[string]cty.Value{
										"machine_type": cty.StringVal("c2-standard-8"),
									}),
								},
							},
						},
					},
				}
			},
			expected: "n2-standard-2,c2-standard-8",
		},
		{
			name: "TPU node type module (schedmd-slurm-gcp-v6-nodeset-tpu)",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Groups: []config.Group{
						{
							Name: config.GroupName("primary"),
							Modules: []config.Module{
								{
									ID:     config.ModuleID("tpu_nodeset"),
									Source: "community/modules/compute/schedmd-slurm-gcp-v6-nodeset-tpu",
									Settings: config.NewDict(map[string]cty.Value{
										"node_type": cty.StringVal("v4-8"),
									}),
								},
							},
						},
					},
				}
			},
			expected: "v4-8",
		},
		{
			name: "No machine types in modules",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Groups: []config.Group{
						{
							Name: config.GroupName("primary"),
							Modules: []config.Module{
								{
									ID:       config.ModuleID("vpc_network"),
									Source:   "modules/network/vpc",
									Settings: config.NewDict(map[string]cty.Value{}),
								},
							},
						},
					},
				}
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := tt.setupBp()

			actual := getMachineType(bp)

			if actual != tt.expected {
				t.Errorf("getMachineType() = %q, want %q", actual, tt.expected)
			}
		})
	}
}

// TestGetOSName verifies that the operating system name correctly matches the runtime GOOS.
func TestGetOSName(t *testing.T) {
	expected := runtime.GOOS
	actual := getOSName()

	if actual != expected {
		t.Errorf("getOSName() = %q, want %q", actual, expected)
	}
}

// TestOSSpecificVersionMethods verifies graceful failure when running
// OS-specific version methods on the wrong OS.
func TestOSSpecificVersionMethods(t *testing.T) {
	linuxVer := getLinuxVersion()
	macVer := getMacVersion()
	winVer := getWindowsVersion()

	if runtime.GOOS != "linux" && linuxVer != "Linux (unknown version)" {
		// Note: On rare occasions (like WSL or specific Mac setups), /etc/os-release
		// might exist, so we use Logf instead of Errorf to avoid flaky tests.
		t.Logf("Unexpected linux version string on %s: %s", runtime.GOOS, linuxVer)
	}

	if runtime.GOOS != "darwin" && macVer != "Darwin (unknown version)" {
		t.Errorf("getMacVersion() = %q on %s, want empty string", macVer, runtime.GOOS)
	}

	if runtime.GOOS != "windows" && winVer != "Windows (unknown version)" {
		t.Errorf("getWindowsVersion() = %q on %s, want empty string", winVer, runtime.GOOS)
	}
}

// TestGetOSVersionDelegation ensures getOSVersion() correctly delegates
// to the right OS-specific method based on runtime.GOOS.
func TestGetOSVersionDelegation(t *testing.T) {
	osVer := getOSVersion()

	switch runtime.GOOS {
	case "linux":
		expected := getLinuxVersion()
		if osVer != expected {
			t.Errorf("getOSVersion() = %q, want %q", osVer, expected)
		}
	case "darwin":
		expected := getMacVersion()
		if osVer != expected {
			t.Errorf("getOSVersion() = %q, want %q", osVer, expected)
		}
	case "windows":
		expected := getWindowsVersion()
		if osVer != expected {
			t.Errorf("getOSVersion() = %q, want %q", osVer, expected)
		}
	default:
		if osVer != "" {
			t.Errorf("getOSVersion() = %q on %s, want empty string", osVer, runtime.GOOS)
		}
	}
}

// TestParseOsReleaseField thoroughly unit tests the string parsing logic
// used by getLinuxVersion to read /etc/os-release files.
func TestParseOsReleaseField(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "With quotes",
			line:     `PRETTY_NAME="Ubuntu 22.04 LTS"`,
			expected: `Ubuntu 22.04 LTS`,
		},
		{
			name:     "Without quotes",
			line:     `VERSION_ID=22.04`,
			expected: `22.04`,
		},
		{
			name:     "No equals sign",
			line:     `INVALID_LINE_FORMAT`,
			expected: ``,
		},
		{
			name:     "Empty value after equals",
			line:     `PRETTY_NAME=`,
			expected: ``,
		},
		{
			name:     "Equals sign in value",
			line:     `PRETTY_NAME="My=Custom=Linux"`,
			expected: `My=Custom=Linux`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := parseOsReleaseField(tt.line)
			if actual != tt.expected {
				t.Errorf("parseOsReleaseField(%q) = %q, want %q", tt.line, actual, tt.expected)
			}
		})
	}
}

func TestGetTerraformVersion(t *testing.T) {
	// Define the test cases
	testCases := []struct {
		name        string
		mockVersion string
		mockError   error
		expected    string
	}{
		{
			name:        "Success - returns terraform version",
			mockVersion: "1.3.7",
			mockError:   nil,
			expected:    "1.3.7",
		},
		{
			name:        "Failure - returns '' on error",
			mockVersion: "",
			mockError:   fmt.Errorf("executable file not found in $PATH"),
			expected:    "",
		},
	}

	// Save the original function and ensure it gets restored after tests
	originalTfVersionFunc := tfVersionFunc
	defer func() { tfVersionFunc = originalTfVersionFunc }()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Inject the mock function for the current test case
			tfVersionFunc = func() (string, error) {
				return tc.mockVersion, tc.mockError
			}

			// 2. Execute the method under test
			actual := getTerraformVersion()

			// 3. Assert the result
			if actual != tc.expected {
				t.Errorf("getTerraformVersion() = %q; want %q", actual, tc.expected)
			}
		})
	}
}

func TestGetIsGke(t *testing.T) {
	tests := []struct {
		name        string
		modulesList []string
		want        string
	}{
		{
			name:        "empty list returns false",
			modulesList: []string{},
			want:        "false",
		},
		{
			name:        "identifies gke-cluster pattern",
			modulesList: []string{"module/network/vpc", "module/gke-cluster/foo"},
			want:        "true",
		},
		{
			name:        "identifies gke-node-pool pattern",
			modulesList: []string{"module/gke-node-pool/bar"},
			want:        "true",
		},
		{
			name:        "returns false when no GKE modules are present",
			modulesList: []string{"module/network/vpc", "module/schedmd-slurm-gcp-v6-controller"},
			want:        "false",
		},
		{
			name:        "handles multiple modules where GKE is present",
			modulesList: []string{"module/network/vpc", "module/gke-cluster/primary", "module/schedmd-slurm-gcp-v6-login"},
			want:        "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getIsGke(tt.modulesList); got != tt.want {
				t.Errorf("getIsGke() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetIsSlurm(t *testing.T) {
	tests := []struct {
		name        string
		modulesList []string
		want        string
	}{
		{
			name:        "empty list returns false",
			modulesList: []string{},
			want:        "false",
		},
		{
			name:        "identifies schedmd-slurm-gcp pattern",
			modulesList: []string{"module/network/vpc", "module/schedmd-slurm-gcp-v6-controller"},
			want:        "true",
		},
		{
			name:        "returns false when no Slurm modules are present",
			modulesList: []string{"module/network/vpc", "module/gke-cluster/foo"},
			want:        "false",
		},
		{
			name:        "handles multiple modules where Slurm is present",
			modulesList: []string{"module/network/vpc", "module/schedmd-slurm-gcp-v6-login", "module/schedmd-slurm-gcp-v6-compute"},
			want:        "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getIsSlurm(tt.modulesList); got != tt.want {
				t.Errorf("getIsSlurm() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetIsVmInstance(t *testing.T) {
	tests := []struct {
		name        string
		modulesList []string
		want        string
	}{
		{
			name:        "empty list returns false",
			modulesList: []string{},
			want:        "false",
		},
		{
			name:        "identifies vm-instance pattern",
			modulesList: []string{"module/network/vpc", "module/vm-instance/compute"},
			want:        "true",
		},
		{
			name:        "returns false when no VM instance modules are present",
			modulesList: []string{"module/network/vpc", "module/gke-cluster/foo", "module/schedmd-slurm-gcp-v6-controller"},
			want:        "false",
		},
		{
			name:        "handles multiple modules where VM instance is present",
			modulesList: []string{"module/vm-instance/login", "module/network/vpc"},
			want:        "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getIsVmInstance(tt.modulesList); got != tt.want {
				t.Errorf("getIsVmInstance() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetProjectNumber verifies that the project number is correctly fetched
// or gracefully fails depending on the blueprint configuration and API response.
func TestGetProjectNumber(t *testing.T) {
	tests := []struct {
		name      string
		blueprint config.Blueprint
		clientErr error
		mockErr   error
		want      string
	}{
		{
			name: "success_1",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal("test-project-1"),
				}),
			},
			want: "1234567890",
		},
		{
			name: "success_2",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal("test-project-2"),
				}),
			},
			want: "9876543210",
		},
		{
			name: "no_project_id",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{}),
			},
			want: "",
		},
		{
			name: "client_creation_error",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal("any-project"),
				}),
			},
			clientErr: errors.New("failed to create client"),
			want:      "",
		},
		{
			name: "api_error",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal("error-project"),
				}),
			},
			mockErr: errors.New("project not found"),
			want:    "",
		},
		{
			name: "api_returns_empty_name",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal(""),
				}),
			},
			want: "",
		},
		{
			name: "api_returns_nil_project",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal("nil-project"),
				}),
			},
			want: "",
		},
		{
			name: "project_id_not_string_type",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.NumberIntVal(123),
				}),
			},
			want: "",
		},
	}

	// To safely mock package-level variables without permanently altering the global state.
	origFetchProjectName := fetchProjectName
	defer func() { fetchProjectName = origFetchProjectName }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the function directly for this test case
			fetchProjectName = func(ctx context.Context, projectID string) (string, error) {
				if tt.clientErr != nil {
					return "", tt.clientErr
				}
				if tt.mockErr != nil {
					return "", tt.mockErr
				}
				// simulate mock responses based on test setup
				if projectID == "test-project-1" {
					return "projects/1234567890", nil
				}
				if projectID == "test-project-2" {
					return "projects/9876543210", nil
				}
				return "", errors.New("not found")
			}

			got := getProjectNumber(tt.blueprint)
			if got != tt.want {
				t.Errorf("getProjectNumber() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetBillingAccountId verifies the extraction and formatting of the billing account ID.
func TestGetBillingAccountId(t *testing.T) {
	// Save the original function and restore it after the test finishes
	originalGetProjectBillingAccount := getProjectBillingAccount
	defer func() { getProjectBillingAccount = originalGetProjectBillingAccount }()

	tests := []struct {
		name               string
		setupBp            func() config.Blueprint
		mockBillingAccount string
		expected           string
	}{
		{
			name: "Missing project_id in blueprint",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{}),
				}
			},
			mockBillingAccount: "",
			expected:           "",
		},
		{
			name: "Project ID present but no billing account returned",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{
						"project_id": cty.StringVal("test-project-123"),
					}),
				}
			},
			mockBillingAccount: "",
			expected:           "",
		},
		{
			name: "Project ID present and billing account trimmed",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{
						"project_id": cty.StringVal("test-project-123"),
					}),
				}
			},
			mockBillingAccount: "billingAccounts/012345-6789AB-CDEF01",
			expected:           "012345-6789AB-CDEF01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the GCP call for this specific test case
			getProjectBillingAccount = func(ctx context.Context, projectID string) string {
				return tt.mockBillingAccount
			}

			bp := tt.setupBp()
			actual := getBillingAccountId(bp)

			if actual != tt.expected {
				t.Errorf("getBillingAccountId() = %q, want %q", actual, tt.expected)
			}
		})
	}
}

func TestGetModules(t *testing.T) {
	// Save and restore the original function to avoid affecting other tests
	originalGetStandardModules := getStandardModules
	defer func() { getStandardModules = originalGetStandardModules }()

	mockStandardList := []string{
		"modules/network/vpc",
		"community/modules/compute/mig",
	}

	tests := []struct {
		name                string
		input               []string
		mockStandardModules []string
		expected            string
	}{
		{
			name:                "success: all standard modules",
			input:               []string{"modules/network/vpc", "community/modules/compute/mig"},
			mockStandardModules: mockStandardList,
			expected:            "modules/network/vpc,community/modules/compute/mig",
		},
		{
			name:                "success: mix of standard and custom modules",
			input:               []string{"modules/network/vpc", "modules/my-custom-network", "community/modules/compute/mig"},
			mockStandardModules: mockStandardList,
			expected:            "modules/network/vpc,Custom,community/modules/compute/mig",
		},
		{
			name:                "success: only custom modules",
			input:               []string{"my/custom/module1", "my/custom/module2"},
			mockStandardModules: mockStandardList,
			expected:            "Custom,Custom",
		},
		{
			name:                "success: empty input",
			input:               []string{},
			mockStandardModules: mockStandardList,
			expected:            "",
		},
		{
			name:                "error: standardModules fetch failed (UNVERIFIED)",
			input:               []string{"modules/network/vpc", "my/custom/module"},
			mockStandardModules: []string{}, // Simulating an empty return indicating fetch failure
			expected:            "UNVERIFIED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Mock the getStandardModules function for this specific test case
			getStandardModules = func() []string {
				return tc.mockStandardModules
			}

			result := getModules(tc.input)

			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

// TestGetIsGoogler tests the full logic of the getIsGoogler method including fallbacks.
func TestGetIsGoogler(t *testing.T) {
	// Save the original environment variables to restore them after the tests
	originalCreds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	originalPath := os.Getenv("PATH")
	defer func() {
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", originalCreds)
		os.Setenv("PATH", originalPath)
	}()

	tempDir := t.TempDir()

	// Helper to create a mock gcloud executable in the temp directory
	createFakeGcloud := func(output string, exitCode int) {
		mockGcloudPath := filepath.Join(tempDir, "gcloud")
		var script string
		if exitCode == 0 {
			script = fmt.Sprintf("#!/bin/sh\necho '%s'\n", output)
		} else {
			script = fmt.Sprintf("#!/bin/sh\nexit %d\n", exitCode)
		}

		err := os.WriteFile(mockGcloudPath, []byte(script), 0755)
		if err != nil {
			t.Fatalf("Failed to write fake gcloud script: %v", err)
		}

		// Prepend the temp directory to the PATH to intercept `exec.Command("gcloud", ...)`
		os.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)
	}

	// Helper to create a fake Application Default Credentials JSON file
	createFakeADC := func(content string) string {
		adcPath := filepath.Join(tempDir, "adc.json")
		err := os.WriteFile(adcPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write fake ADC file: %v", err)
		}
		return adcPath
	}

	tests := []struct {
		name           string
		setupADC       bool
		adcContent     string
		gcloudOutput   string
		gcloudExitCode int
		expected       bool
	}{
		{
			name:       "Success - Internal ADC is present",
			setupADC:   true,
			adcContent: `{"client_email": "test-sa@hpc-toolkit-dev.iam.gserviceaccount.com"}`,
			expected:   true,
		},
		{
			name:       "Success - Internal ADC project number is present",
			setupADC:   true,
			adcContent: `{"client_email": "508417052821@cloudbuild.gserviceaccount.com"}`,
			expected:   true,
		},
		{
			name:           "Success - Fallback to gcloud when ADC is external",
			setupADC:       true,
			adcContent:     `{"client_email": "external@example.com"}`,
			gcloudOutput:   "user@google.com",
			gcloudExitCode: 0,
			expected:       true,
		},
		{
			name:           "Success - Fallback to gcloud when ADC is invalid",
			setupADC:       true,
			adcContent:     `{invalid_json}`,
			gcloudOutput:   "user@google.com",
			gcloudExitCode: 0,
			expected:       true,
		},
		{
			name:           "Success - Internal user via gcloud directly (No ADC)",
			setupADC:       false,
			gcloudOutput:   "user@google.com",
			gcloudExitCode: 0,
			expected:       true,
		},
		{
			name:           "Failure - External user via gcloud directly (No ADC)",
			setupADC:       false,
			gcloudOutput:   "user@example.com",
			gcloudExitCode: 0,
			expected:       false,
		},
		{
			name:           "Failure - External ADC and gcloud fails execution",
			setupADC:       true,
			adcContent:     `{"client_email": "external@example.com"}`,
			gcloudExitCode: 1, // Represents a failure when running the CLI
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ADC file if required
			if tt.setupADC {
				adcPath := createFakeADC(tt.adcContent)
				os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adcPath)
			} else {
				os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
			}

			// Mock gcloud command
			createFakeGcloud(tt.gcloudOutput, tt.gcloudExitCode)

			got := getIsGoogler()
			if got != tt.expected {
				t.Errorf("getIsGoogler() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestCheckADCForInternalUser tests the JSON file parsing logic for Application Default Credentials.
func TestCheckADCForInternalUser(t *testing.T) {
	tempDir := t.TempDir()

	createADC := func(content string) string {
		path := filepath.Join(tempDir, "adc.json")
		_ = os.WriteFile(path, []byte(content), 0644)
		return path
	}

	tests := []struct {
		name        string
		content     string
		fileExists  bool
		expected    bool
		expectError bool
	}{
		{
			name:        "Valid internal ADC user",
			content:     `{"client_email": "test-sa@hpc-toolkit-dev.iam.gserviceaccount.com"}`,
			fileExists:  true,
			expected:    true,
			expectError: false,
		},
		{
			name:        "Valid external ADC user",
			content:     `{"client_email": "external@example.com"}`,
			fileExists:  true,
			expected:    false,
			expectError: false,
		},
		{
			name:        "Malformed JSON payload",
			content:     `{invalid}`,
			fileExists:  true,
			expected:    false,
			expectError: true,
		},
		{
			name:        "ADC file missing",
			fileExists:  false,
			expected:    false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			if tt.fileExists {
				path = createADC(tt.content)
			} else {
				path = filepath.Join(tempDir, "non_existent_adc.json")
			}

			got, err := checkADCForInternalUser(path)
			if (err != nil) != tt.expectError {
				t.Errorf("checkADCForInternalUser() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if got != tt.expected {
				t.Errorf("checkADCForInternalUser() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestIsInternalEmail tests the allowlisting and domain verification logic.
func TestIsInternalEmail(t *testing.T) {
	tests := []struct {
		email    string
		expected bool
	}{
		// Positive test cases
		{"user@google.com", true},
		{"user@sub.google.com", true},
		{"test-sa@hpc-toolkit-dev.iam.gserviceaccount.com", true},
		{"test-sa@hpc-toolkit-demo.iam.gserviceaccount.com", true},
		{"test-sa@hpc-toolkit-gsc.dev.gserviceaccount.com", true},
		{"508417052821@cloudbuild.gserviceaccount.com", true},
		{"858831239249.foo.@cloudbuild.gserviceaccount.com", true},
		{"266450182917@cloudbuild.gserviceaccount.com", true},
		// Negative test cases
		{"user@example.com", false},
		{"test-sa@other-external-project.iam.gserviceaccount.com", false},
		{"1234567890@cloudbuild.gserviceaccount.com", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := isInternalEmail(tt.email); got != tt.expected {
				t.Errorf("isInternalEmail(%q) = %v, want %v", tt.email, got, tt.expected)
			}
		})
	}
}
