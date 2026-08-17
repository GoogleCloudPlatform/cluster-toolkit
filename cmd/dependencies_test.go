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
	"testing"

	"hpc-toolkit/pkg/dependencies"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestAddDependenciesFlags(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	addDependenciesFlags(flags)

	flag := flags.Lookup("download-dependencies")
	if flag == nil {
		t.Fatalf("Expected 'download-dependencies' flag to be added, but it was not")
	}

	if flag.DefValue != "false" {
		t.Errorf("Expected default value to be 'false', got '%s'", flag.DefValue)
	}
}

func TestCheckDependencies(t *testing.T) {
	cmd := &cobra.Command{Use: "test-command"}
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	addDependenciesFlags(flags)
	cmd.Flags().AddFlagSet(flags)

	called := false
	var capturedTools []string
	originalFn := ensureDependenciesFn
	ensureDependenciesFn = func(d dependencies.DownloadDecision, tools ...string) error {
		called = true
		capturedTools = tools
		return nil
	}
	defer func() { ensureDependenciesFn = originalFn }()

	checkDependencies(cmd, "terraform")

	if !called {
		t.Errorf("Expected ensureDependenciesFn to be called")
	}
	if len(capturedTools) != 1 || capturedTools[0] != "terraform" {
		t.Errorf("Expected captured tools to be ['terraform'], got %v", capturedTools)
	}
}
