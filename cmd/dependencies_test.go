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

func TestInitDependenciesIgnoresCommands(t *testing.T) {
	cmd := &cobra.Command{Use: "unrelated-command"}
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	addDependenciesFlags(flags)
	cmd.Flags().AddFlagSet(flags)

	called := false
	originalFn := ensureDependenciesFn
	ensureDependenciesFn = func(d dependencies.DownloadDecision) error {
		called = true
		return nil
	}
	defer func() { ensureDependenciesFn = originalFn }()

	initDependencies(cmd)

	if called {
		t.Errorf("Expected ensureDependenciesFn not to be called")
	}
}

func TestInitDependenciesAllowedCommands(t *testing.T) {
	for _, cmdName := range []string{"deploy", "destroy", "export-outputs"} {
		cmd := &cobra.Command{Use: cmdName}
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		addDependenciesFlags(flags)
		cmd.Flags().AddFlagSet(flags)

		called := false
		originalFn := ensureDependenciesFn
		ensureDependenciesFn = func(d dependencies.DownloadDecision) error {
			called = true
			return nil
		}
		defer func() { ensureDependenciesFn = originalFn }()

		initDependencies(cmd)

		if !called {
			t.Errorf("Expected ensureDependenciesFn to be called for command %s", cmdName)
		}
	}
}
