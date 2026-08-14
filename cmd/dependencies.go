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
	"hpc-toolkit/pkg/dependencies"
	"hpc-toolkit/pkg/logging"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var downloadDependencies bool
var ensureDependenciesFn = dependencies.EnsureDependencies

func addDependenciesFlags(flagset *pflag.FlagSet) {
	flagset.BoolVar(&downloadDependencies, "download-dependencies", false, "Automatically download missing dependencies. Pass --download-dependencies=false to fail if missing.")
}

func checkDependencies(cmd *cobra.Command, tools ...string) {
	decision := dependencies.DownloadDecisionAsk
	if cmd.Flags().Changed("download-dependencies") {
		if downloadDependencies {
			decision = dependencies.DownloadDecisionYes
		} else {
			decision = dependencies.DownloadDecisionNo
		}
	}

	if err := ensureDependenciesFn(decision, tools...); err != nil {
		logging.Fatal("Failed to setup dependencies: %v", err)
	}
}
