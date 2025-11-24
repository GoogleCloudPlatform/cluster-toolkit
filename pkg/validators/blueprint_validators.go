// Copyright 2025 "Google LLC"
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
)

// ValidateBlueprint runs fast, deterministic, blueprint-level checks that do NOT
// depend on outputs from other groups. Return a slice of errors (empty => ok).
func ValidateBlueprint(bp config.Blueprint) []error {
	var errs []error
	// Only run Slurm checks on Slurm blueprints.
	if e := checkSlurmNodeCount(bp); e != nil {
		errs = append(errs, e)
	}
	if e := checkSlurmProvisioning(bp); e != nil {
		errs = append(errs, e)
	}
	if e := checkSlurmClusterName(bp); e != nil {
		errs = append(errs, e)
	}

	// Add further checks here in the desired order, using the same pattern:
	// if e := someOtherCheck(bp); e != nil { return []error{e} }

	// No error
	return errs
}
