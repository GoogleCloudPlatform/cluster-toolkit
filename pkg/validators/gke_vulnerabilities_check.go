// Copyright 2026 "Google LLC"
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
	_ "embed"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/telemetry"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

//go:embed security-advisories.json
var securityAdvisoriesJSON []byte

type Advisory struct {
	CVE             string            `json:"cve"`
	Name            string            `json:"name"`
	AffectedImages  []string          `json:"affected_images"`
	Status          string            `json:"status"` // "PATCHED" or "PENDING"
	PatchedVersions map[string]string `json:"patched_versions"`
	Link            string            `json:"link"`
}

type VulnerabilityDB struct {
	Advisories []Advisory `json:"advisories"`
}

// PerformGkeVersionSecurityChecks
func PerformGkeVulnerabilitiesCheck(cmd *cobra.Command, args []string) {
	skipSecurity, _ := cmd.Flags().GetBool("skip-gke-security-check")

	// 3. evaluate security vulnerabilities BEFORE Terraform starts
	if !skipSecurity {
		blueprint := telemetry.GetBlueprint(cmd, args)
		gkeVersions, err := config.ResolveGKEVersions(&blueprint)
		if err != nil {
			logging.Info("Error resolving GKE version from blueprint: %v", err)
		}
		if len(gkeVersions) > 0 {
			db, err := fetchAdvisories()
			if err != nil {
				logging.Info("Could not fetch security advisories: %v", err)
			} else {
				warnings := evaluate(db, gkeVersions)

				// If vulnerabilities are found, print them
				if len(warnings) > 0 {
					for _, w := range warnings {
						logging.Info("%v", w)
					}
				}
			}
		}
	}
}

// fetchAdvisories attempts to read the Security Advisories data stored in the repo.
func fetchAdvisories() (*VulnerabilityDB, error) {
	var db VulnerabilityDB

	if err := json.Unmarshal(securityAdvisoriesJSON, &db); err != nil {
		return nil, fmt.Errorf("failed to parse Security advisories: %v", err)
	}

	return &db, nil
}

// evaluate checks the blueprint's GKE configuration against known advisories.
func evaluate(db *VulnerabilityDB, gkeVersions []string) []string {
	var warnings []string
	for _, gkeVersion := range gkeVersions {

		// Normalize version string for semver evaluation
		if !strings.HasPrefix(gkeVersion, "v") {
			gkeVersion = "v" + gkeVersion
		}

		minorVersion := semver.MajorMinor(gkeVersion)

		for _, adv := range db.Advisories {
			switch adv.Status {
			case "PENDING":
				warnings = append(warnings, fmt.Sprintf(
					"SECURITY WARNING: Your deployment is vulnerable to %s (%s). "+
						"Patches are currently PENDING in upstream GKE. See: %s",
					adv.CVE, adv.Name, adv.Link))
			case "PATCHED":
				if patchedVersion, exists := adv.PatchedVersions[minorVersion]; exists {
					// Normalize patched version format
					if !strings.HasPrefix(patchedVersion, "v") {
						patchedVersion = "v" + patchedVersion
					}
					if semver.Compare(gkeVersion, patchedVersion) < 0 {
						warnings = append(warnings, fmt.Sprintf(
							"SECURITY WARNING: Your GKE version %s is vulnerable to %s (%s). "+
								"Please upgrade your blueprint to at least %s. See: %s",
							gkeVersion, adv.CVE, adv.Name, patchedVersion, adv.Link))
					}
				}
			}
		}
	}

	return warnings
}
