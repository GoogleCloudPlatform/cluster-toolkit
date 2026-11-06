/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/telemetry"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/spf13/cobra"
	. "gopkg.in/check.v1"
)

type MySuite struct{}

var _ = Suite(&MySuite{})

var randomGitHash = "a975c295ddeab5b1a5323df92f61c4cc9fc88207"

func Test(t *testing.T) {
	TestingT(t)
}

func TestMain(m *testing.M) {
	wd, _ := os.Getwd()
	code := m.Run()
	os.Chdir(wd)
	os.Exit(code)
}

/* Tests */
// root.go
func (s *MySuite) TestHpcToolkitRepo(c *C) {
	path := c.MkDir()
	repo, initHash, err := initTestRepo(path)
	if err != nil {
		c.Fatal(err)
	}
	GitInitialHash = initHash.String()
	head, _ := repo.Head()

	{ // CWD is repo root
		if err = os.Chdir(path); err != nil {
			c.Fatal(err)
		}
		r, dir, err := hpcToolkitRepo()
		c.Assert(err, IsNil)
		checkPathsEqual(c, dir, path)
		h, _ := r.Head()
		c.Check(h.Hash(), Equals, head.Hash())
	}

	{ // CWD is subdir in repo root
		subDir := filepath.Join(path, "subdir")
		if err = os.MkdirAll(subDir, os.ModePerm); err != nil {
			c.Fatal(err)
		}
		if err = os.Chdir(subDir); err != nil {
			c.Fatal(err)
		}
		r, dir, err := hpcToolkitRepo()
		c.Assert(err, IsNil)
		checkPathsEqual(c, dir, subDir)
		h, _ := r.Head()
		c.Check(h.Hash(), Equals, head.Hash())
	}

	{ // CWD is root of sub repo in repo root
		subRepo := filepath.Join(path, "subrepo")
		if err = os.MkdirAll(subRepo, os.ModePerm); err != nil {
			c.Fatal(err)
		}
		_, _, err = initTestRepo(subRepo)
		if err != nil {
			c.Fatal(err)
		}
		if err = os.Chdir(subRepo); err != nil {
			c.Fatal(err)
		}
		r, dir, err := hpcToolkitRepo()
		c.Assert(err, IsNil)
		checkPathsEqual(c, dir, path)
		h, _ := r.Head()
		c.Check(h.Hash(), Equals, head.Hash())
	}

	{ // CWD is parent of repo root, hope it's not repo itself.
		os.Chdir(filepath.Dir(path))
		_, _, err = hpcToolkitRepo()
		c.Check(err, Equals, git.ErrRepositoryNotExists)
	}
}

func (s *MySuite) TestIsHpcToolkitRepo(c *C) {
	repo, initHash, err := initTestRepo(c.MkDir())
	if err != nil {
		c.Fatal(err)
	}

	// Doesn't match
	GitInitialHash = randomGitHash
	c.Check(isHpcToolkitRepo(*repo), Equals, false)

	// Matches
	GitInitialHash = initHash.String()
	c.Check(isHpcToolkitRepo(*repo), Equals, true)
}

func (s *MySuite) TestCheckGitHashMismatch(c *C) {
	path := c.MkDir()
	repo, init, err := initTestRepo(path)
	if err != nil {
		c.Fatal(err)
	}
	if err = os.Chdir(path); err != nil {
		c.Fatal(err)
	}

	head, _ := repo.Head()
	hash := head.Hash().String()
	branch := head.Name().Short()

	{ // Matches
		GitInitialHash = init.String()
		GitCommitHash = hash
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, false)
		c.Check(b, Equals, "")
		c.Check(h, Equals, "")
		c.Check(dir, Equals, "")
	}

	{ // Baked commit hash doesn't match (present in repo, but not HEAD)
		GitInitialHash = init.String()
		GitCommitHash = init.String()
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, true)
		c.Check(b, Equals, branch)
		c.Check(h, Equals, hash)
		checkPathsEqual(c, dir, path)
	}

	{ // Baked commit hash doesn't match (not present in repo)
		GitInitialHash = init.String()
		GitCommitHash = randomGitHash
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, true)
		c.Check(b, Equals, branch)
		c.Check(h, Equals, hash)
		checkPathsEqual(c, dir, path)
	}

	{ // Not a right repo, initial hash doesn't match
		GitInitialHash = randomGitHash
		GitCommitHash = randomGitHash
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, false)
		c.Check(b, Equals, "")
		c.Check(h, Equals, "")
		c.Check(dir, Equals, "")
	}

	{ // Binary contains no git information
		GitTagVersion = ""
		GitBranch = ""
		GitCommitInfo = ""
		GitCommitHash = ""
		GitInitialHash = ""
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, false)
		c.Check(b, Equals, "")
		c.Check(h, Equals, "")
		c.Check(dir, Equals, "")
	}
}

func checkPathsEqual(c *C, a, b string) {
	a, err := filepath.EvalSymlinks(a)
	if err != nil {
		c.Fatal(err)
	}
	b, err = filepath.EvalSymlinks(b)
	if err != nil {
		c.Fatal(err)
	}
	c.Check(a, Equals, b)
}

// Creates a Git repo at `path`, performs multiple commits.
func initTestRepo(path string) (repo *git.Repository, initHash plumbing.Hash, err error) {
	fs := osfs.New(path)
	storer := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	repo, err = git.Init(storer, fs)
	if err != nil {
		return
	}
	w, err := repo.Worktree()
	if err != nil {
		return
	}

	commit := func(s string) (hash plumbing.Hash, err error) {
		n := "test_" + s + ".txt"
		f, err := fs.Create(n)
		if err != nil {
			return
		}
		// Set unique content to avoid hash collision
		f.Write([]byte(s + " @ " + path))
		f.Close()
		w.Add(n)
		hash, err = w.Commit(s, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "T T",
				Email: "t@t.io",
				When:  time.Now(),
			},
		})
		return
	}

	initHash, err = commit("Init")
	if err != nil {
		return
	}
	_, err = commit("Last")
	return
}

func TestExecute_TelemetryAndErrorHandling(t *testing.T) {
	// Isolate the config directory for tests to prevent modifying the real user config file
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("APPDATA", tempDir) // For Windows compatibility
	t.Setenv("HOME", tempDir)    // For macOS/Linux compatibility

	// Ensure config is initialized and telemetry is explicitly turned OFF to prevent actual network/telemetry calls during unit tests.
	_ = config.InitUserConfig()
	_ = config.SetTelemetry(false)
	// Reset the flag at the start of the test block
	telemetryFlushed.Store(false)

	// Create a real *exec.ExitError for testing the exit code unwrapping logic
	execCmd := exec.Command("false")
	exitErr := execCmd.Run()

	tests := []struct {
		name        string
		setupCmd    func() *cobra.Command
		expectError bool
		expectedErr error
	}{
		{
			name: "Successful command execution",
			setupCmd: func() *cobra.Command {
				return &cobra.Command{
					Use: "dummy-success",
					RunE: func(cmd *cobra.Command, args []string) error {
						return nil // No error
					},
				}
			},
			expectError: false,
		},
		{
			name: "Generic error execution",
			setupCmd: func() *cobra.Command {
				return &cobra.Command{
					Use: "dummy-error",
					RunE: func(cmd *cobra.Command, args []string) error {
						return errors.New("generic error")
					},
				}
			},
			expectError: true,
			expectedErr: errors.New("generic error"),
		},
		{
			name: "ExitError execution",
			setupCmd: func() *cobra.Command {
				return &cobra.Command{
					Use: "dummy-exit-error",
					RunE: func(cmd *cobra.Command, args []string) error {
						return exitErr // Should unwrap to a specific exit code internally
					},
				}
			},
			expectError: true,
			expectedErr: exitErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global state initialized by PersistentPreRun to ensure clean slates
			userConfigExists = false
			telemetryCollector = nil

			// Reset the flag between subtests
			telemetryFlushed.Store(false)

			// Inject a dummy command to trigger specific error cases
			dummyCmd := tt.setupCmd()
			rootCmd.AddCommand(dummyCmd)

			// Set args to invoke the dummy command
			rootCmd.SetArgs([]string{dummyCmd.Use})

			// Invoke the package-level Execute function
			err := Execute()

			// 1. Verify Error Handling Passthrough
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected an error but got nil")
				} else if err.Error() != tt.expectedErr.Error() {
					t.Errorf("Expected error '%v', got '%v'", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			// 2. Verify PersistentPreRun Initialization Logic
			if !userConfigExists {
				t.Errorf("Expected userConfigExists to be true, got false. PersistentPreRun did not execute.")
			}
			if telemetryCollector == nil {
				t.Errorf("Expected telemetryCollector to be initialized, got nil.")
			}

			// Cleanup dummy command for the next iteration
			rootCmd.RemoveCommand(dummyCmd)
		})
	}
}

func TestFatalHook_ConcurrencyAndReentrancy(t *testing.T) {
	// 1. Isolate config and reset state
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	_ = config.InitUserConfig()
	_ = config.SetTelemetry(false) // Prevent actual telemetry network calls during the test

	// Reset the atomic flag to its default state for a clean test
	telemetryFlushed.Store(false)
	telemetryCollector = nil

	// 2. Execute PersistentPreRun to register the FatalHook
	rootCmd.PersistentPreRun(rootCmd, []string{})

	if logging.FatalHook == nil {
		t.Fatal("Expected logging.FatalHook to be registered by PersistentPreRun")
	}

	// 3. Simulate multiple concurrent calls to logging.FatalHook
	var wg sync.WaitGroup
	numCalls := 10

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// This simulates multiple goroutines hitting logging.Fatal concurrently,
			// or a recursive logging.Fatal call inside the collector.
			logging.FatalHook(1, nil)
		}()
	}

	// Wait for all concurrent executions to finish
	wg.Wait()

	// 4. Verify that the atomic flag was flipped to true and handled concurrency safely
	if !telemetryFlushed.Load() {
		t.Errorf("Expected telemetryFlushed to be true after FatalHook execution")
	}
}

func TestWrapTelemetryDynamic(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	_ = config.InitUserConfig()

	telemetryCollector = nil
	userConfigExists = false

	// Create a command with a custom hook
	customCmd := &cobra.Command{
		Use: "custom",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	// Wrap it
	wrapTelemetry(customCmd)

	// Execute the hook
	err := customCmd.PersistentPreRunE(customCmd, []string{})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Assert telemetry was initialized
	if telemetryCollector == nil {
		t.Errorf("Expected telemetryCollector to be initialized by wrapTelemetry")
	}
	if !userConfigExists {
		t.Errorf("Expected userConfigExists to be true")
	}
}

func TestWrapTelemetryInitializesBeforePreRunError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	_ = config.InitUserConfig()

	telemetryCollector = nil
	userConfigExists = false

	preRunErr := errors.New("missing --cluster flag")
	customCmd := &cobra.Command{
		Use: "job-test",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return preRunErr
		},
	}

	wrapTelemetry(customCmd)

	err := customCmd.PersistentPreRunE(customCmd, []string{})
	if !errors.Is(err, preRunErr) {
		t.Fatalf("Expected error %v, got %v", preRunErr, err)
	}
	if telemetryCollector == nil {
		t.Errorf("Expected telemetryCollector to be initialized even when PersistentPreRunE fails")
	}
	if !userConfigExists {
		t.Errorf("Expected userConfigExists to be true even when PersistentPreRunE fails")
	}
}

func TestMergeDeploymentSettingsPropagatesAIAssisted(t *testing.T) {
	bp := config.Blueprint{}
	ds := config.DeploymentSettings{AIAssisted: true}
	if err := mergeDeploymentSettings(&bp, ds); err != nil {
		t.Fatalf("mergeDeploymentSettings failed: %v", err)
	}
	if !bp.AIAssisted {
		t.Errorf("Expected bp.AIAssisted to be true when ds.AIAssisted is true")
	}
}

func TestApplyAgentEnvAttribution(t *testing.T) {
	t.Setenv("GHPC_AI_ASSISTED", "")
	t.Setenv("GEMINI_CLI", "1")

	expandBP := config.Blueprint{}
	applyAgentEnvAttribution(expandCmd, &expandBP)
	if expandBP.AIAssisted {
		t.Errorf("Expected gcluster expand to keep untouched blueprint pure (AIAssisted=false)")
	}

	createBP := config.Blueprint{}
	applyAgentEnvAttribution(createCmd, &createBP)
	if !createBP.AIAssisted {
		t.Errorf("Expected gcluster create to persist AIAssisted=true under agent env")
	}

	deployBP := config.Blueprint{}
	applyAgentEnvAttribution(deployCmd, &deployBP)
	if !deployBP.AIAssisted {
		t.Errorf("Expected gcluster deploy to persist AIAssisted=true under agent env")
	}
}

func TestExpandOrDie_AgentEnvDoesNotChangeLabels(t *testing.T) {
	dir := t.TempDir()
	bpPath := filepath.Join(dir, "bp.yaml")
	content := "blueprint_name: test-bp\nvars:\n  project_id: test-project\n  deployment_name: test-deployment\n"
	if err := os.WriteFile(bpPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write blueprint: %v", err)
	}
	origLevel := expandFlags.validationLevel
	expandFlags.validationLevel = "IGNORE"
	defer func() { expandFlags.validationLevel = origLevel }()

	t.Setenv("GHPC_AI_ASSISTED", "")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLOUDSDK_CORE_ACCOUNT", "testuser")

	bp, _ := expandOrDie(createCmd, bpPath)
	if !bp.AIAssisted {
		t.Errorf("expected bp.AIAssisted to be true for telemetry after expandOrDie in create command")
	}
	labels := bp.Vars.Get("labels").AsValueMap()
	if _, exists := labels["ghpc_ai_assisted"]; exists {
		t.Errorf("expected env-only detection not to inject ghpc_ai_assisted into vars.labels, got %v", labels["ghpc_ai_assisted"])
	}
}

func setupIsolatedExecuteTest(t *testing.T) (string, func()) {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_CACHE_HOME", tempDir)
	t.Setenv("APPDATA", tempDir)
	t.Setenv("HOME", tempDir)
	t.Setenv("GHPC_AI_ASSISTED", "false")

	_ = config.InitUserConfig()
	_ = config.SetTelemetry(false)

	origSilenceErrors := rootCmd.SilenceErrors
	origSilenceUsage := rootCmd.SilenceUsage
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	t.Cleanup(func() {
		rootCmd.SilenceErrors = origSilenceErrors
		rootCmd.SilenceUsage = origSilenceUsage
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	reset := func() {
		userConfigExists = false
		telemetryCollector = nil
		telemetryFlushed.Store(false)
		outBuf := new(bytes.Buffer)
		rootCmd.SetOut(outBuf)
		rootCmd.SetErr(outBuf)
	}
	return tempDir, reset
}

func TestExecute_PreRunErrors(t *testing.T) {
	tempDir, resetTelemetryState := setupIsolatedExecuteTest(t)

	aiDeplPath := filepath.Join(tempDir, "ai-depl.yaml")
	if err := os.WriteFile(aiDeplPath, []byte("ai_assisted: true\nvars:\n  project_id: test-p\n"), 0644); err != nil {
		t.Fatalf("failed writing ai-depl.yaml: %v", err)
	}

	t.Run("Unknown subcommand initializes fallback telemetry", func(t *testing.T) {
		resetTelemetryState()
		t.Setenv("GHPC_AI_ASSISTED", "")
		t.Setenv("GEMINI_CLI", "1")
		rootCmd.SetArgs([]string{"nonexistent-plan-cmd"})

		err := Execute()
		if err == nil || telemetryCollector == nil {
			t.Fatalf("Expected error and initialized telemetryCollector, got err=%v collector=%v", err, telemetryCollector)
		}
		telemetryCollector.CollectMetrics(1, err)
		if got, _ := telemetryCollector.GetMetadata(telemetry.IS_AI_ASSISTED); got != "true" {
			t.Errorf("IS_AI_ASSISTED = %q, want %q", got, "true")
		}
	})

	t.Run("Unknown flag and checkExists error initialize fallback telemetry", func(t *testing.T) {
		dummy := &cobra.Command{
			Use:  "dummy-create <BLUEPRINT_FILE>",
			Args: cobra.MatchAll(cobra.ExactArgs(1), checkExists),
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		dummy.Flags().StringP("deployment-file", "d", "", "")
		rootCmd.AddCommand(dummy)
		defer rootCmd.RemoveCommand(dummy)

		resetTelemetryState()
		t.Setenv("GHPC_AI_ASSISTED", "false")
		missingBP := filepath.Join(tempDir, "missing-blueprint.yaml")
		rootCmd.SetArgs([]string{"dummy-create", missingBP, "-d", aiDeplPath})

		err := Execute()
		if err == nil || telemetryCollector == nil {
			t.Fatalf("Expected checkExists error and initialized collector, got err=%v collector=%v", err, telemetryCollector)
		}
		telemetryCollector.CollectMetrics(1, err)
		if got, _ := telemetryCollector.GetMetadata(telemetry.IS_AI_ASSISTED); got != "true" {
			t.Errorf("IS_AI_ASSISTED = %q, want %q from -d deployment.yaml", got, "true")
		}
		if got, _ := telemetryCollector.GetMetadata(telemetry.ERROR_TYPE); got != telemetry.ErrTypeYamlDoesNotExist {
			t.Errorf("ERROR_TYPE = %q, want %q", got, telemetry.ErrTypeYamlDoesNotExist)
		}

		resetTelemetryState()
		t.Setenv("GHPC_AI_ASSISTED", "")
		t.Setenv("CLAUDECODE", "1")
		rootCmd.SetArgs([]string{"dummy-create", "--unknown-dry-run-flag"})

		if err := Execute(); err == nil || telemetryCollector == nil {
			t.Fatalf("Expected unknown flag error and initialized collector, got err=%v collector=%v", err, telemetryCollector)
		}
	})
}

func TestExecute_PreRunTelemetryGuards(t *testing.T) {
	_, resetTelemetryState := setupIsolatedExecuteTest(t)

	t.Run("Help flag does not initialize telemetry", func(t *testing.T) {
		dummy := &cobra.Command{
			Use:  "dummy-help-target",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		rootCmd.AddCommand(dummy)
		defer rootCmd.RemoveCommand(dummy)

		resetTelemetryState()
		rootCmd.SetArgs([]string{"dummy-help-target", "--help"})

		if err := Execute(); err != nil {
			t.Fatalf("Unexpected error on --help: %v", err)
		}
		if telemetryCollector != nil {
			t.Errorf("Expected telemetryCollector to stay nil on --help")
		}
	})

	t.Run("Telemetry opt-out and __complete commands skip fallback telemetry", func(t *testing.T) {
		resetTelemetryState()
		rootCmd.SetArgs([]string{"telemetry"})
		if err := Execute(); err == nil {
			t.Fatal("Expected ExactArgs error for bare 'telemetry' command")
		}
		if telemetryCollector != nil || userConfigExists {
			t.Errorf("Expected telemetry opt-out command error to skip fallback telemetry")
		}

		hiddenComplete := &cobra.Command{
			Use:    "__complete-test",
			Hidden: true,
			Args:   cobra.ExactArgs(1),
			RunE:   func(cmd *cobra.Command, args []string) error { return nil },
		}
		rootCmd.AddCommand(hiddenComplete)
		defer rootCmd.RemoveCommand(hiddenComplete)

		resetTelemetryState()
		rootCmd.SetArgs([]string{"__complete-test"})
		if err := Execute(); err == nil {
			t.Fatal("Expected ExactArgs error for __complete-test")
		}
		if telemetryCollector != nil {
			t.Errorf("Expected __complete command error to skip fallback telemetry")
		}
	})
}
