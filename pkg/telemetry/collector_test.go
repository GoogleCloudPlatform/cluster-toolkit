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
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zclconf/go-cty/cty"
)

func TestNewCollector(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	// Passing nil for args prevents getBlueprint from attempting to read a file
	c := NewCollector(cmd, nil)

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
		IS_TEST_DATA,
		EXIT_CODE,
	}

	tests := []struct {
		name           string
		errorCode      int
		setupCmd       func(cmd *cobra.Command) // Hook to configure the command
		setupCollector func(c *Collector)       // Hook to mock internal collector state
		expectedValues map[string]string
	}{
		{
			name:      "Success exit code",
			errorCode: 0,
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
				IS_TEST_DATA:  "true",
				EXIT_CODE:     "0",
				COMMAND_FLAGS: "force,project",
				REGION:        "us-central1",
				ZONE:          "us-central1-a",
				MACHINE_TYPE:  "c2-standard-8",
				OS_NAME:       getOSName(),    // Dynamically expect the current OS name
				OS_VERSION:    getOSVersion(), // Dynamically expect the current OS version
			},
		},
		{
			name:      "Failure exit code with missing region, zone, and machine type",
			errorCode: 1,
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
				IS_TEST_DATA:  "true",
				EXIT_CODE:     "1",
				COMMAND_FLAGS: "",
				REGION:        "",
				ZONE:          "",
				OS_NAME:       getOSName(),    // Verify OS info is still collected on failure
				OS_VERSION:    getOSVersion(), // Verify OS info is still collected on failure
				MACHINE_TYPE:  "",             // Verify empty machine type when no matching modules exist
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
			c := NewCollector(cmd, []string{})

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

	c := NewCollector(childCmd, nil)
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

// TestHelperProcess isn't a regular test. It acts as a dummy executable
// to mock the output of exec.Command during unit testing.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("MOCK_GCLOUD_OUTPUT"))
	if os.Getenv("MOCK_GCLOUD_ERR") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestGetIsGoogler(t *testing.T) {
	// Save the original functions to restore them after the test
	originalExecCommand := execCommand
	originalExecLookPath := execLookPath
	defer func() {
		execCommand = originalExecCommand
		execLookPath = originalExecLookPath
	}()

	tests := []struct {
		name         string
		gcloudEmail  string
		gcloudFail   bool
		mockBinaries map[string]bool
		expected     bool
	}{
		{
			name:         "google account with @google.com domain",
			gcloudEmail:  "user@google.com\n",
			gcloudFail:   false,
			mockBinaries: map[string]bool{},
			expected:     true,
		},
		{
			name:         "non-google account, but gcert is present",
			gcloudEmail:  "user@example.com\n",
			gcloudFail:   false,
			mockBinaries: map[string]bool{"gcert": true},
			expected:     true,
		},
		{
			name:         "non-google account, but prodaccess is present",
			gcloudEmail:  "user@example.com\n",
			gcloudFail:   false,
			mockBinaries: map[string]bool{"prodaccess": true},
			expected:     true,
		},
		{
			name:         "non-google account, no internal binaries",
			gcloudEmail:  "user@example.com\n",
			gcloudFail:   false,
			mockBinaries: map[string]bool{},
			expected:     false,
		},
		{
			name:         "gcloud command fails, but gcert is present",
			gcloudEmail:  "",
			gcloudFail:   true,
			mockBinaries: map[string]bool{"gcert": true},
			expected:     true,
		},
		{
			name:         "gcloud command fails, no internal binaries",
			gcloudEmail:  "",
			gcloudFail:   true,
			mockBinaries: map[string]bool{},
			expected:     false,
		},
		{
			name:         "empty gcloud output, no internal binaries",
			gcloudEmail:  "",
			gcloudFail:   false,
			mockBinaries: map[string]bool{},
			expected:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Apply our mocks for this test case
			execCommand = mockExecCommand(tc.gcloudEmail, tc.gcloudFail)
			execLookPath = mockLookPath(tc.mockBinaries)

			actual := getIsGoogler()
			if actual != tc.expected {
				t.Errorf("getIsGoogler() = %v, expected %v", actual, tc.expected)
			}
		})
	}
}

// mockExecCommand creates a mock function to replace exec.Command
func mockExecCommand(output string, fail bool) func(string, ...string) *exec.Cmd {
	return func(command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		cmd.Env = append(cmd.Env, fmt.Sprintf("MOCK_GCLOUD_OUTPUT=%s", output))
		if fail {
			cmd.Env = append(cmd.Env, "MOCK_GCLOUD_ERR=1")
		}
		return cmd
	}
}

// mockLookPath creates a mock function to replace exec.LookPath
func mockLookPath(mockBinaries map[string]bool) func(string) (string, error) {
	return func(file string) (string, error) {
		if mockBinaries[file] {
			return "/mock/path/to/" + file, nil
		}
		return "", errors.New("executable file not found in $PATH")
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
