package validators

import (
	"hpc-toolkit/pkg/config"
)

// isSlurmBlueprint checks if the blueprint contains the Slurm controller module.
func isSlurmBlueprint(bp config.Blueprint) bool {
	for _, group := range bp.Groups {
		for _, module := range group.Modules {
			if module.Source == "community/modules/scheduler/schedmd-slurm-gcp-v6-controller" {
				return true
			}
		}
	}
	return false
}

// ValidateBlueprint runs fast, deterministic, blueprint-level checks that do NOT
// depend on outputs from other groups. Return a slice of errors (empty => ok).
func ValidateBlueprint(bp config.Blueprint) []error {
	// Only run Slurm checks on Slurm blueprints.
	if isSlurmBlueprint(bp) {
		if e := checkSlurmNodeCount(bp); e != nil {
			return []error{e}
		}
		if e := checkSlurmProvisioning(bp); e != nil {
			return []error{e}
		}
		if e := checkSlurmClusterName(bp); e != nil {
			return []error{e}
		}
	}

	// Add further checks here in the desired order, using the same pattern:
	// if e := someOtherCheck(bp); e != nil { return []error{e} }

	// No error
	return nil
}
