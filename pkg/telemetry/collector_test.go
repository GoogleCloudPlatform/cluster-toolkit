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
	"bytes"
	"context"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
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
		STATIC_NODE_COUNTS,
		OS_NAME,
		OS_VERSION,
		TERRAFORM_VERSION,
		INSTALLATION_MODE,
		IS_TEST_DATA,
		EXIT_CODE,
	}

	tests := []struct {
		name             string
		errorCode        int
		err              error
		installationMode string
		setupCmd         func(cmd *cobra.Command) // Hook to configure the command
		setupCollector   func(c *Collector)       // Hook to mock internal collector state
		expectedValues   map[string]string
	}{
		{
			name:             "Success exit code",
			errorCode:        0,
			err:              nil,
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
									ID:     config.ModuleID("compute_pool"),
									Source: "modules/compute/vm-instance",
									Settings: config.NewDict(map[string]cty.Value{
										"machine_type":   cty.StringVal("c2-standard-8"),
										"instance_count": cty.NumberIntVal(1),
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
				STATIC_NODE_COUNTS: "c2-standard-8:1",
				OS_NAME:            getOSName(),           // Dynamically expect the current OS name
				OS_VERSION:         getOSVersion(),        // Dynamically expect the current OS version
				TERRAFORM_VERSION:  getTerraformVersion(), // Dynamically expect the current Terraform version
				INSTALLATION_MODE:  SOURCE,
			},
		},
		{
			name:             "Failure exit code with missing region, zone, and machine type",
			errorCode:        1,
			err:              nil,
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
				STATIC_NODE_COUNTS: "",
				INSTALLATION_MODE:  BINARY,
			},
		},
		{
			name:             "Failure exit code with error",
			errorCode:        1,
			err:              errors.New("permission denied error"),
			installationMode: SOURCE,
			setupCmd: func(cmd *cobra.Command) {
			},
			setupCollector: func(c *Collector) {
				c.blueprint = config.Blueprint{
					Vars:   config.NewDict(map[string]cty.Value{}),
					Groups: []config.Group{},
				}
			},
			expectedValues: map[string]string{
				EXIT_CODE:  "1",
				ERROR_TYPE: ErrTypePermissionDenied,
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
			c.CollectMetrics(tt.errorCode, tt.err)

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

func TestGetBlueprint(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// 1. Create a dummy base blueprint
	bpPath := filepath.Join(tmpDir, "blueprint.yaml")
	bpContent := []byte(`
blueprint_name: test-blueprint
vars:
  project_id: default-project
  region: us-central1
`)
	_ = os.WriteFile(bpPath, bpContent, 0644)

	// 2. Create a dummy deployment file for override testing
	depFile := filepath.Join(tmpDir, "deployment.yaml")
	depContent := []byte(`
vars:
  project_id: deployment-file-project
`)
	_ = os.WriteFile(depFile, depContent, 0644)

	// 3. Create a dummy deployment directory with an artifacts folder
	deployDir := filepath.Join(tmpDir, "my-deployment")
	artifactsDir := modulewriter.ArtifactsDir(deployDir)
	_ = os.MkdirAll(artifactsDir, 0755)
	expandedBpPath := filepath.Join(artifactsDir, "expanded_blueprint.yaml")
	expandedBpContent := []byte(`
blueprint_name: expanded-test-blueprint
vars:
  project_id: artifacts-project
`)
	_ = os.WriteFile(expandedBpPath, expandedBpContent, 0644)

	tests := []struct {
		name          string
		args          []string
		setupCmd      func() *cobra.Command
		expectIsEmpty bool
		expectedName  string
		expectedVars  map[string]string // Key to expected variable value
	}{
		{
			name:          "No arguments",
			args:          []string{},
			setupCmd:      func() *cobra.Command { return &cobra.Command{} },
			expectIsEmpty: true,
		},
		{
			name:         "Basic blueprint file",
			args:         []string{bpPath},
			setupCmd:     func() *cobra.Command { return &cobra.Command{} },
			expectedName: "test-blueprint",
			expectedVars: map[string]string{"project_id": "default-project"},
		},
		{
			name:         "Deployment directory",
			args:         []string{deployDir},
			setupCmd:     func() *cobra.Command { return &cobra.Command{} },
			expectedName: "expanded-test-blueprint",
			expectedVars: map[string]string{"project_id": "artifacts-project"},
		},
		{
			name: "With deployment-file override",
			args: []string{bpPath},
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("deployment-file", depFile, "")
				return cmd
			},
			expectedName: "test-blueprint",
			expectedVars: map[string]string{"project_id": "deployment-file-project"},
		},
		{
			name: "With vars flag override",
			args: []string{bpPath},
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().StringSlice("vars", []string{"project_id=cli-project", "zone=us-east1-a"}, "")
				return cmd
			},
			expectedName: "test-blueprint",
			expectedVars: map[string]string{
				"project_id": "cli-project",
				"zone":       "us-east1-a",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.setupCmd()
			bp := getBlueprint(cmd, tc.args)

			if tc.expectIsEmpty {
				// Assert that the returned Blueprint is effectively empty
				if bp.BlueprintName != "" || len(bp.Groups) > 0 {
					t.Errorf("Expected an empty blueprint, but got: %+v", bp)
				}
			} else {
				// Assert that the returned Blueprint parsed the correct file
				if bp.BlueprintName != tc.expectedName {
					t.Errorf("Expected BlueprintName %q, got %q", tc.expectedName, bp.BlueprintName)
				}

				// Assert that variables were properly overridden/merged
				for k, expectedVal := range tc.expectedVars {
					if !bp.Vars.Has(k) {
						t.Errorf("Expected variable %q to exist, but it was missing", k)
						continue
					}

					// Evaluate the variable to safely extract its string value
					val := bp.Vars.Get(k)
					unmarked, _ := val.Unmark()
					if unmarked.AsString() != expectedVal {
						t.Errorf("Expected variable %q to be %q, got %q", k, expectedVal, unmarked.AsString())
					}
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
	c.CollectMetrics(0, nil)

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
	// Mock the ID generator to always return the dummy value
	originalGen := config.GenerateUniqueIDFunc
	config.GenerateUniqueIDFunc = func() string {
		return "a1b2c3d4e5f6"
	}
	// Ensure it gets restored after the test finishes
	defer func() { config.GenerateUniqueIDFunc = originalGen }()

	tests := []struct {
		name       string
		mockConfig func(configPath string)
		want       string
	}{
		{
			name: "returns valid client install id when set in config",
			mockConfig: func(configPath string) {
				// Mocks the case where the CLI has already bootstrapped the config
				// and saved a persistent user_id.
				content := `{"user_id": "a1b2c3d4e5f6"}`
				_ = os.MkdirAll(filepath.Dir(configPath), 0755)
				_ = os.WriteFile(configPath, []byte(content), 0644)
			},
			want: "a1b2c3d4e5f6",
		},
		{
			name: "recreates and returns client install id when it is explicitly set to empty string",
			mockConfig: func(configPath string) {
				// Mocks the case where the config has been corrupted or emptied
				content := `{"user_id": ""}`
				_ = os.MkdirAll(filepath.Dir(configPath), 0755)
				_ = os.WriteFile(configPath, []byte(content), 0644)
			},
			want: "a1b2c3d4e5f6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up a clean temporary directory for the config to avoid polluting the host
			tempDir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", tempDir) // Linux
			t.Setenv("HOME", tempDir)            // macOS / Linux fallback
			t.Setenv("AppData", tempDir)         // Windows
			t.Setenv("LocalAppData", tempDir)    // Windows fallback

			configPath := filepath.Join(tempDir, "cluster-toolkit", "telemetry_config.json")

			// Apply the mocked configuration file for this specific test case
			tt.mockConfig(configPath)

			// Initialize the config to load the mocked JSON file into the new globalUserConfig state
			err := config.InitUserConfig()
			if err != nil {
				t.Fatalf("Failed to initialize user config in test: %v", err)
			}

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

// TestGetMachineType verifies that machine types are correctly extracted from the blueprint.
func TestGetMachineType(t *testing.T) {
	tests := []struct {
		name string
		bp   config.Blueprint
		want string
	}{
		{
			name: "Extracts explicit machine_type",
			bp: config.Blueprint{
				Groups: []config.Group{
					{
						Name: config.GroupName("primary"),
						Modules: []config.Module{
							{
								ID: config.ModuleID("compute_node"),
								Settings: config.NewDict(map[string]cty.Value{
									"machine_type": cty.StringVal("c2-standard-8"),
								}),
							},
						},
					},
				},
			},
			want: "c2-standard-8",
		},
		{
			name: "Extracts explicit node_type (TPU)",
			bp: config.Blueprint{
				Groups: []config.Group{
					{
						Name: config.GroupName("primary"),
						Modules: []config.Module{
							{
								ID: config.ModuleID("tpu_node"),
								Settings: config.NewDict(map[string]cty.Value{
									"node_type": cty.StringVal("v4-8"),
								}),
							},
						},
					},
				},
			},
			want: "v4-8",
		},
		{
			name: "Extracts explicit system_node_pool_machine_type (GKE)",
			bp: config.Blueprint{
				Groups: []config.Group{
					{
						Name: config.GroupName("primary"),
						Modules: []config.Module{
							{
								ID: config.ModuleID("gke_cluster"),
								Settings: config.NewDict(map[string]cty.Value{
									"system_node_pool_machine_type": cty.StringVal("e2-standard-16"),
								}),
							},
						},
					},
				},
			},
			want: "e2-standard-16",
		},
		{
			name: "Extracts default machine_type when omitted from blueprint",
			bp: config.Blueprint{
				Groups: []config.Group{
					{
						Name: config.GroupName("primary"),
						Modules: []config.Module{
							{
								ID: config.ModuleID("controller_node"),
								// The real embedded controller module has a default `machine_type: "c2-standard-4"`
								Source: "../../community/modules/scheduler/schedmd-slurm-gcp-v6-controller",
								Kind:   config.TerraformKind,
							},
						},
					},
				},
			},
			want: "c2-standard-4",
		},
		{
			name: "Deduplicates matching machine types across modules",
			bp: config.Blueprint{
				Groups: []config.Group{
					{
						Name: config.GroupName("primary"),
						Modules: []config.Module{
							{
								ID: config.ModuleID("node1"),
								Settings: config.NewDict(map[string]cty.Value{
									"machine_type": cty.StringVal("c2-standard-8"),
								}),
							},
							{
								ID: config.ModuleID("node2"),
								Settings: config.NewDict(map[string]cty.Value{
									"machine_type": cty.StringVal("c2-standard-8"),
								}),
							},
							{
								ID: config.ModuleID("node3_tpu"),
								Settings: config.NewDict(map[string]cty.Value{
									"node_type": cty.StringVal("c2-standard-8"),
								}),
							},
						},
					},
				},
			},
			want: "c2-standard-8",
		},
		{
			name: "Returns empty string if no types are found",
			bp: config.Blueprint{
				Groups: []config.Group{
					{
						Name: config.GroupName("primary"),
						Modules: []config.Module{
							{
								ID:     config.ModuleID("vpc"),
								Source: "../../modules/network/vpc",
								Kind:   config.TerraformKind,
								Settings: config.NewDict(map[string]cty.Value{
									"some_other_setting": cty.StringVal("c2-standard-8"),
								}),
							},
						},
					},
				},
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getMachineType(tc.bp)
			if got != tc.want {
				t.Errorf("getMachineType() = %q; want %q", got, tc.want)
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

// TestGetProjectNumber verifies that the project number is correctly fetched or gracefully fails depending on the API response.
func TestGetProjectNumber(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		clientErr error
		mockErr   error
		want      string
	}{
		{
			name:      "success_1",
			projectID: "test-project-1",
			want:      "1234567890",
		},
		{
			name:      "success_2",
			projectID: "test-project-2",
			want:      "9876543210",
		},
		{
			name:      "no_project_id",
			projectID: "",
			want:      "",
		},
		{
			name:      "client_creation_error",
			projectID: "any-project",
			clientErr: errors.New("failed to create client"),
			want:      "",
		},
		{
			name:      "api_error",
			projectID: "error-project",
			mockErr:   errors.New("project not found"),
			want:      "",
		},
		{
			name:      "api_returns_empty_name",
			projectID: "empty-name-project",
			want:      "",
		},
		{
			name:      "api_returns_nil_project",
			projectID: "nil-project",
			want:      "",
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
				if projectID == "empty-name-project" || projectID == "nil-project" {
					return "", nil
				}
				return "", errors.New("not found")
			}

			got := getProjectNumber(tt.projectID)
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
		projectID          string
		mockBillingAccount string
		mockErr            error
		expected           string
	}{
		{
			name:               "Missing project_id",
			projectID:          "",
			mockBillingAccount: "",
			expected:           "",
		},
		{
			name:               "Project ID present but no billing account returned",
			projectID:          "test-project-123",
			mockBillingAccount: "",
			expected:           "",
		},
		{
			name:               "Project ID present and billing account trimmed",
			projectID:          "test-project-123",
			mockBillingAccount: "billingAccounts/012345-6789AB-CDEF01",
			expected:           "012345-6789AB-CDEF01",
		},
		{
			name:               "Project ID present but API fails",
			projectID:          "test-project-123",
			mockBillingAccount: "",
			mockErr:            errors.New("api error"),
			expected:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the GCP call for this specific test case, now returning (string, error)
			getProjectBillingAccount = func(ctx context.Context, projectID string) (string, error) {
				return tt.mockBillingAccount, tt.mockErr
			}

			actual := getBillingAccountId(tt.projectID)

			if actual != tt.expected {
				t.Errorf("getBillingAccountId() = %q, want %q", actual, tt.expected)
			}
		})
	}
}

func TestGetModules(t *testing.T) {
	// Save and restore the original transport to not pollute other tests
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	// Mock JSON response that config.GetPredefinedModules() will parse
	mockJSON := `{
		"tree": [
			{"path": "modules/network/vpc/main.tf", "type": "blob"},
			{"path": "community/modules/compute/mig/main.pkr.hcl", "type": "blob"}
		]
	}`

	tests := []struct {
		name     string
		input    []string
		mockResp *http.Response
		expected string
	}{
		{
			name:  "success: all standard modules",
			input: []string{"modules/network/vpc", "community/modules/compute/mig"},
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockJSON)),
			},
			// Expected to keep original paths
			expected: "modules/network/vpc,community/modules/compute/mig",
		},
		{
			name:  "success: mix of standard and custom modules",
			input: []string{"modules/network/vpc", "modules/my-custom-network", "community/modules/compute/mig"},
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockJSON)),
			},
			// Expected to sanitize the unknown module
			expected: "modules/network/vpc,Custom,community/modules/compute/mig",
		},
		{
			name:  "success: only custom modules",
			input: []string{"my/custom/module1", "my/custom/module2"},
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockJSON)),
			},
			// Expected to sanitize all
			expected: "Custom,Custom",
		},
		{
			name:  "success: empty input",
			input: []string{},
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockJSON)),
			},
			expected: "",
		},
		{
			name:  "error: standardModules fetch failed (UNVERIFIED)",
			input: []string{"modules/network/vpc", "my/custom/module"},
			mockResp: &http.Response{
				Status:     "500 Internal Server Error",
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Internal Server Error"}`)),
			},
			expected: "UNVERIFIED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Force the OS cache directories to a clean, temporary folder
			// so that cache files don't leak between test cases.
			tempDir := t.TempDir()
			t.Setenv("XDG_CACHE_HOME", tempDir) // Linux
			t.Setenv("HOME", tempDir)           // macOS / Linux fallback
			t.Setenv("LocalAppData", tempDir)   // Windows

			// Inject our HTTP mock response for this test case
			http.DefaultTransport = &mockTransport{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					return tc.mockResp, nil
				},
			}

			result := getModules(tc.input)

			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetBlueprintName(t *testing.T) {
	// Save and restore the original transport so we don't pollute other tests
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTreeJSON := `{
		"tree": [
			{"path": "examples/hpc-slurm.yaml", "type": "blob"}
		]
	}`

	tests := []struct {
		name     string
		input    config.Blueprint
		mockResp *http.Response
		mockYaml string
		expected string
	}{
		{
			name:  "success: identifies standard blueprint name",
			input: config.Blueprint{BlueprintName: "hpc-slurm"},
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockTreeJSON)),
			},
			mockYaml: "blueprint_name: hpc-slurm",
			expected: "hpc-slurm",
		},
		{
			name:  "success: sanitizes unrecognized blueprint to Custom",
			input: config.Blueprint{BlueprintName: "my-secret-cluster"},
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockTreeJSON)),
			},
			mockYaml: "blueprint_name: hpc-slurm", // The server only knows about "hpc-slurm"
			expected: "Custom",
		},
		{
			name:  "success: empty blueprint name returns early",
			input: config.Blueprint{BlueprintName: ""},
			// Network mock is not needed because the function returns before fetching
			expected: "",
		},
		{
			name:  "error: fetch failure safely falls back to UNVERIFIED",
			input: config.Blueprint{BlueprintName: "hpc-slurm"},
			mockResp: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Internal Server Error"}`)),
			},
			expected: "UNVERIFIED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Force OS Cache Directories to a clean, temporary folder to prevent test leakage
			tempDir := t.TempDir()
			t.Setenv("XDG_CACHE_HOME", tempDir)
			t.Setenv("HOME", tempDir)
			t.Setenv("LocalAppData", tempDir)

			// Set up our mock HTTP routing
			http.DefaultTransport = &mockTransport{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					url := req.URL.String()

					// Route 1: GitHub API Tree Request
					if strings.Contains(url, "/git/trees/") {
						// If the test case expects an error (like 500), return it immediately
						if tc.mockResp != nil && tc.mockResp.StatusCode != http.StatusOK {
							return tc.mockResp, nil
						}
						// Otherwise, return the standard successful tree
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBufferString(mockTreeJSON)),
						}, nil
					}

					// Route 2: Raw GitHub YAML Request
					if strings.Contains(url, "hpc-slurm.yaml") && tc.mockYaml != "" {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBufferString(tc.mockYaml)),
						}, nil
					}

					return &http.Response{
						StatusCode: http.StatusNotFound,
						Body:       io.NopCloser(bytes.NewBufferString("404 Not Found")),
					}, nil
				},
			}

			result := getBlueprintName(tc.input)

			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetDeploymentFile(t *testing.T) {
	// Save and restore the original transport to not pollute other tests
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockJSON := `{
		"tree": [
			{"path": "examples/hpc-slurm.yaml", "type": "blob"},
			{"path": "community/examples/ml-cluster.yml", "type": "blob"}
		]
	}`

	tests := []struct {
		name       string
		flagValue  string
		flagExists bool
		mockResp   *http.Response
		expected   string
	}{
		{
			name:       "success: exact match standard file",
			flagValue:  "community/examples/ml-cluster.yml",
			flagExists: true,
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockJSON)),
			},
			expected: "community/examples/ml-cluster.yml",
		},
		{
			name:       "success: custom file",
			flagValue:  "my-custom-cluster.yaml",
			flagExists: true,
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockJSON)),
			},
			expected: "Custom",
		},
		{
			name:       "success: custom parent directory",
			flagValue:  "my-directory/examples/hpc-slurm.yaml",
			flagExists: true,
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockJSON)),
			},
			expected: "examples/hpc-slurm.yaml",
		},
		{
			name:       "success: windows path normalization",
			flagValue:  ".\\examples\\hpc-slurm.yaml",
			flagExists: true,
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockJSON)),
			},
			expected: "examples/hpc-slurm.yaml",
		},
		{
			name:       "success: flag not set",
			flagValue:  "",
			flagExists: false,
			mockResp:   nil,
			expected:   "",
		},
		{
			name:       "success: flag exists but is empty string",
			flagValue:  "",
			flagExists: true,
			mockResp:   nil,
			expected:   "",
		},
		{
			name:       "error: standard files fetch failed",
			flagValue:  "examples/hpc-slurm.yaml",
			flagExists: true,
			mockResp: &http.Response{
				Status:     "500 Internal Server Error",
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Internal Server Error"}`)),
			},
			expected: "UNVERIFIED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Force the OS cache directories to a clean, temporary folder
			// so that cache files don't leak between test cases.
			tempDir := t.TempDir()
			t.Setenv("XDG_CACHE_HOME", tempDir)
			t.Setenv("HOME", tempDir)
			t.Setenv("LocalAppData", tempDir)

			if tc.mockResp != nil {
				// Inject our HTTP mock response for this test case
				http.DefaultTransport = &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tc.mockResp, nil
					},
				}
			}

			// Set up a mock Cobra command
			cmd := &cobra.Command{Use: "test"}
			if tc.flagExists {
				cmd.Flags().String("deployment-file", tc.flagValue, "")
				// Parse to simulate the user passing the flag
				_ = cmd.ParseFlags([]string{"--deployment-file", tc.flagValue})
			}

			result := getDeploymentFile(cmd)

			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

// TestEvaluateIsGoogler covers the core logic of determining Googler status,
// maintaining all 4 original edge cases from the legacy subprocess tests.
func TestEvaluateIsGoogler(t *testing.T) {
	tests := []struct {
		name         string
		setupADC     bool
		adcContent   string
		gcloudOutput string
		gcloudFail   bool
		expected     bool
	}{
		{
			name:       "Failure - External ADC and gcloud fails execution",
			setupADC:   true,
			adcContent: `{"client_email": "external@example.com"}`,
			gcloudFail: true, // Represents a failure when reading gcloud config files
			expected:   false,
		},
		{
			name:         "Failure - External user via gcloud directly (No ADC)",
			setupADC:     false,
			gcloudOutput: "user@example.com",
			gcloudFail:   false,
			expected:     false,
		},
		{
			name:         "Success - Internal user via gcloud directly (No ADC)",
			setupADC:     false,
			gcloudOutput: "user@google.com",
			gcloudFail:   false,
			expected:     true,
		},
		{
			name:         "Success - Fallback to gcloud when ADC is invalid",
			setupADC:     true,
			adcContent:   `{invalid_json}`,
			gcloudOutput: "user@google.com",
			gcloudFail:   false,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Mock ADC file if required
			if tt.setupADC {
				adcPath := filepath.Join(t.TempDir(), "adc.json")
				_ = os.WriteFile(adcPath, []byte(tt.adcContent), 0644)
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adcPath)
			} else {
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
			}

			// 2. Mock gcloud config directory via CLOUDSDK_CONFIG
			gcloudDir := t.TempDir()
			t.Setenv("CLOUDSDK_CONFIG", gcloudDir)

			if !tt.gcloudFail {
				activeConfigPath := filepath.Join(gcloudDir, "active_config")
				_ = os.WriteFile(activeConfigPath, []byte("default"), 0644)

				configDir := filepath.Join(gcloudDir, "configurations")
				_ = os.MkdirAll(configDir, 0755)

				configFile := filepath.Join(configDir, "config_default")
				iniContent := "[core]\naccount = " + tt.gcloudOutput + "\n"
				_ = os.WriteFile(configFile, []byte(iniContent), 0644)
			}

			// 3. Evaluate the result
			result := evaluateIsGoogler()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestGetIsGoogler_Cache verifies the wrapper method respects the cached value
// in the global config without re-evaluating.
func TestGetIsGoogler_Cache(t *testing.T) {
	// Setup isolated config environment
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir) // Linux
	t.Setenv("HOME", tempDir)            // macOS
	t.Setenv("AppData", tempDir)         // Windows

	// Clear environments so evaluateIsGoogler would definitely return false if it ran
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_CONFIG", t.TempDir())

	// Initialize user config for the test
	err := config.InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// 1. Manually set the cache to true
	_ = config.SetIsGoogler(true)

	// 2. Call getIsGoogler; it should return true despite having no valid ADC/gcloud config
	result := getIsGoogler()
	if !result {
		t.Errorf("Expected getIsGoogler to use cached true value")
	}

	// 3. Change cache to false and verify it reflects the update
	_ = config.SetIsGoogler(false)
	result = getIsGoogler()
	if result {
		t.Errorf("Expected getIsGoogler to use cached false value")
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

// TestCheckGcloudConfigForInternalUser_Success verifies INI parsing correctly identifies an internal user.
func TestCheckGcloudConfigForInternalUser_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", tempDir)

	// 1. Create active_config file
	activeConfigPath := filepath.Join(tempDir, "active_config")
	_ = os.WriteFile(activeConfigPath, []byte("my_test_config\n"), 0644)

	// 2. Create the configuration file
	configDir := filepath.Join(tempDir, "configurations")
	_ = os.MkdirAll(configDir, 0755)

	configFile := filepath.Join(configDir, "config_my_test_config")
	iniContent := `
[core]
account = testuser@google.com
project = test-project

[compute]
zone = us-central1-a
`
	_ = os.WriteFile(configFile, []byte(iniContent), 0644)

	// 3. Evaluate
	result := checkGcloudConfigForInternalUser()
	if !result {
		t.Errorf("Expected checkGcloudConfigForInternalUser to return true for internal email")
	}
}

// TestCheckGcloudConfigForInternalUser_External verifies INI parsing correctly rejects an external user.
func TestCheckGcloudConfigForInternalUser_External(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", tempDir)

	activeConfigPath := filepath.Join(tempDir, "active_config")
	_ = os.WriteFile(activeConfigPath, []byte("default"), 0644)

	configDir := filepath.Join(tempDir, "configurations")
	_ = os.MkdirAll(configDir, 0755)

	configFile := filepath.Join(configDir, "config_default")
	iniContent := `
[core]
account = externaluser@example.com
`
	_ = os.WriteFile(configFile, []byte(iniContent), 0644)

	result := checkGcloudConfigForInternalUser()
	if result {
		t.Errorf("Expected checkGcloudConfigForInternalUser to return false for external email")
	}
}

// TestCheckGcloudConfigForInternalUser_MissingFiles verifies the function fails gracefully if files don't exist.
func TestCheckGcloudConfigForInternalUser_MissingFiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", tempDir) // Empty dir

	result := checkGcloudConfigForInternalUser()
	if result {
		t.Errorf("Expected checkGcloudConfigForInternalUser to return false when config files are missing")
	}
}

func TestGetErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "Nil Error",
			err:      nil,
			expected: "",
		},
		{
			name:     "Permission Denied",
			err:      os.ErrPermission,
			expected: ErrTypePermissionDenied,
		},
		{
			name:     "Resource Not Exist",
			err:      os.ErrNotExist,
			expected: ErrTypeResourceNotFound,
		},
		{
			name:     "Context Deadline Exceeded",
			err:      context.DeadlineExceeded,
			expected: ErrTypeTimeout,
		},
		{
			name:     "Context Canceled",
			err:      context.Canceled,
			expected: ErrTypeCanceled,
		},
		{
			name:     "Text Match Validation",
			err:      errors.New("invalid argument provided"),
			expected: ErrTypeValidation,
		},
		{
			name:     "Text Match Network",
			err:      errors.New("failed to dial tcp: connection refused"),
			expected: ErrTypeNetwork,
		},
		{
			name:     "Text Match Permission",
			err:      errors.New("server responded with 403 forbidden"),
			expected: ErrTypePermissionDenied,
		},
		{
			name:     "Text Match Not Found",
			err:      errors.New("resource not found"),
			expected: ErrTypeResourceNotFound,
		},
		{
			name:     "Unknown Error",
			err:      errors.New("something went entirely wrong"),
			expected: ErrTypeUnknown,
		},
		{
			name:     "Text Match Quota",
			err:      errors.New("google api error: quota exceeded for c2-standard-8"),
			expected: ErrTypeQuotaExceeded,
		},
		{
			name:     "Text Match Auth",
			err:      errors.New("unauthorized request to remote server"),
			expected: ErrTypeAuthentication,
		},
		{
			name:     "Text Match Provisioning",
			err:      errors.New("deployment failed to finish"),
			expected: ErrTypeProvisioning,
		},
		{
			name:     "Text Match Stockout",
			err:      errors.New("A c2-standard-60 VM instance is currently unavailable"),
			expected: ErrTypeStockout,
		},
		{
			name:     "Text Match APIDisabled",
			err:      errors.New("Cloud Filestore API has not been used in project 12345 before or it is disabled."),
			expected: ErrTypeAPIDisabled,
		},
		{
			name:     "Text Match ResourceAlreadyExists",
			err:      errors.New("googleapi: Error 409: Resource already exists"),
			expected: ErrTypeResourceExists,
		},
		{
			name:     "Capitalization Test",
			err:      errors.New("PERMISSION DENIED TO ACCESS THIS RESOURCE"),
			expected: ErrTypePermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getErrorType(tt.err); got != tt.expected {
				t.Errorf("getErrorType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestGetStaticNodeCounts verifies that static node counts are correctly extracted from the blueprint.
func TestGetStaticNodeCounts(t *testing.T) {
	tests := []struct {
		name string
		bp   config.Blueprint
		want string
	}{
		{
			// 1. Standard Top-Level Module Definition
			// The code matches the key, extracts the integer, and maps it directly to the machine type.
			name: "Standard top-level module definition",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("pool1"),
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type":      cty.StringVal("g4"),
							"static_node_count": cty.NumberIntVal(3),
						}),
					}},
				}},
			},
			want: "g4:3",
		},
		{
			// 2. Multiple Modules Sharing a Machine Type
			// Two modules defining the same machine type have their node counts successfully aggregated into a single total.
			name: "Multiple modules sharing a machine type",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{
						{
							ID: config.ModuleID("pool1"),
							Settings: config.NewDict(map[string]cty.Value{
								"machine_type":   cty.StringVal("c2-standard-8"),
								"instance_count": cty.NumberIntVal(2),
							}),
						},
						{
							ID: config.ModuleID("pool2"),
							Settings: config.NewDict(map[string]cty.Value{
								"machine_type":   cty.StringVal("c2-standard-8"),
								"instance_count": cty.NumberIntVal(4),
							}),
						},
					},
				}},
			},
			want: "c2-standard-8:6",
		},
		{
			// 3. Multiple Modules with Varying Machine Types
			// Maps multiple types into distinct keys and deterministically sorts them alphabetically.
			name: "Multiple modules with varying machine types",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{
						{
							ID: config.ModuleID("pool1"),
							Settings: config.NewDict(map[string]cty.Value{
								"machine_type":      cty.StringVal("g4"),
								"static_node_count": cty.NumberIntVal(3),
							}),
						},
						{
							ID: config.ModuleID("pool2"),
							Settings: config.NewDict(map[string]cty.Value{
								"machine_type":      cty.StringVal("a3u"),
								"node_count_static": cty.NumberIntVal(2),
							}),
						},
					},
				}},
			},
			want: "a3u:2,g4:3",
		},
		{
			// 4. Explicit Zero Override
			// Explicitly passing a 0 node count safely flags as found, avoiding a fallback to default values.
			name: "Explicit zero override safely evaluates and avoids default fallback",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("pool1"),
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type":   cty.StringVal("n1-standard-4"),
							"instance_count": cty.NumberIntVal(0),
						}),
					}},
				}},
			},
			want: "",
		},
		{
			// 5. Autoscaling Parameters Co-Existing
			// Autoscaling bounds are ignored because they are excluded from the target keys check.
			name: "Autoscaling parameters co-existing with static counts",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("pool1"),
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type":               cty.StringVal("n2-standard-2"),
							"static_node_count":          cty.NumberIntVal(2),
							"autoscaling_max_node_count": cty.NumberIntVal(10), // Should be ignored
						}),
					}},
				}},
			},
			want: "n2-standard-2:2",
		},
		{
			// 6. Inline Slurm Partitions with Inherited Machine Types
			// Inline lists of objects successfully inherit the top-level machine type when omitted inside the block.
			name: "Inline Slurm partitions with inherited machine types",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("slurm_partition"),
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type": cty.StringVal("c2-standard-8"),
							"nodeset": cty.ListVal([]cty.Value{
								cty.ObjectVal(map[string]cty.Value{
									"node_count_static": cty.NumberIntVal(5),
								}),
							}),
						}),
					}},
				}},
			},
			want: "c2-standard-8:5",
		},
		{
			// 7. Heterogeneous Inline Slurm Partitions
			// Individual machine types correctly override top-level designations within nested heterogeneous blocks.
			name: "Heterogeneous inline Slurm partitions override top-level machine types",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("slurm_partition_mixed"),
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type": cty.StringVal("default-type"), // Ignored due to inline override
							"partition": cty.ListVal([]cty.Value{
								cty.ObjectVal(map[string]cty.Value{
									"machine_type":      cty.StringVal("a2-highgpu-1g"),
									"node_count_static": cty.NumberIntVal(2),
								}),
								cty.ObjectVal(map[string]cty.Value{
									"machine_type":      cty.StringVal("a3-ultragpu-8g"),
									"node_count_static": cty.NumberIntVal(4),
								}),
							}),
						}),
					}},
				}},
			},
			want: "a2-highgpu-1g:2,a3-ultragpu-8g:4",
		},
		{
			// 8. Module Missing a Machine Type Definition
			// Evaluates to an empty machine type string, causing the parsing function to skip counting the module entirely.
			name: "Module missing a machine type definition",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("network_module"),
						Settings: config.NewDict(map[string]cty.Value{
							"instance_count": cty.NumberIntVal(5),
						}),
					}},
				}},
			},
			want: "",
		},
		{
			// 9. Unknown or Computed Variables
			// Detects values that are not known until the apply phase, flagging them as found but resolving to 0 to prevent panics.
			name: "Unknown or computed variables do not panic and evaluate safely",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("pool1"),
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type":      cty.StringVal("g4"),
							"static_node_count": cty.UnknownVal(cty.Number),
						}),
					}},
				}},
			},
			want: "",
		},
		{
			// 10. Blueprints with No Compute Instances
			// Safe fast-path execution returns an empty string entirely when no modules populate the accumulator map.
			name: "Blueprints with no compute instances",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name:    config.GroupName("primary"),
					Modules: []config.Module{},
				}},
			},
			want: "",
		},
		{
			// 11. (Additional) Extraneous Non-Numeric Nested Data
			// Validates that encountering strings or booleans within expected numeric targets cleanly falls back without breaking.
			name: "Extraneous non-numeric nested data is safely ignored",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("slurm_partition_extraneous"),
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type": cty.StringVal("c2-standard-8"),
							"nodeset": cty.TupleVal([]cty.Value{
								cty.ObjectVal(map[string]cty.Value{
									"node_count_static": cty.StringVal("invalid-string-should-be-ignored"),
								}),
								cty.ObjectVal(map[string]cty.Value{
									"node_count_static": cty.NumberIntVal(3),
								}),
							}),
						}),
					}},
				}},
			},
			want: "c2-standard-8:3",
		},
		{
			// 12. TPU Topology calculation
			// Validates that TPU node count is calculated correctly using the topology string when static_node_count is missing.
			name: "TPU topology automatically infers static_node_count",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("tpu_pool"),
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type": cty.StringVal("ct5lp-hightpu-8t"),
							"tpu_topology": cty.StringVal("8x16"),
						}),
					}},
				}},
			},
			want: "ct5lp-hightpu-8t:16",
		},
		{
			// 13. Multiplier properties (num_node_pools, num_slices)
			// Validates that node count multiplies correctly when num_node_pools and num_slices are present.
			name: "num_node_pools and num_slices multiply the base count",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Name: config.GroupName("primary"),
					Modules: []config.Module{{
						ID: config.ModuleID("multi_pool"),
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type":      cty.StringVal("n2-standard-4"),
							"static_node_count": cty.NumberIntVal(2),
							"num_node_pools":    cty.NumberIntVal(3),
							"num_slices":        cty.NumberIntVal(4),
						}),
					}},
				}},
			},
			want: "n2-standard-4:8", // 2 * max(3, 4) = 8
		},
		{
			name: "Extracts target_size as static_node_count",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Modules: []config.Module{{
						Source: "modules/compute/htcondor-execute-point",
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type": cty.StringVal("e2-standard-2"),
							"target_size":  cty.NumberIntVal(10),
						}),
					}},
				}},
			},
			want: "e2-standard-2:10",
		},
		{
			name: "Zonal multiplier applies to static_node_count when zones are declared",
			bp: config.Blueprint{
				Groups: []config.Group{{
					Modules: []config.Module{{
						Source: "modules/compute/gke-node-pool",
						Settings: config.NewDict(map[string]cty.Value{
							"machine_type":      cty.StringVal("t2a-standard-1"),
							"static_node_count": cty.NumberIntVal(2),
							"zones":             cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c")}),
						}),
					}},
				}},
			},
			want: "t2a-standard-1:6", // 2 count * 3 zones
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getStaticNodeCounts(tc.bp)
			if got != tc.want {
				t.Errorf("getStaticNodeCounts() = %q; want %q", got, tc.want)
			}
		})
	}
}
