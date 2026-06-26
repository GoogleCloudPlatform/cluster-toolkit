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
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"

	"golang.org/x/mod/semver"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
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

// PerformGkeVulnerabilitiesCheck checks the GKE version of the cluster against known vulnerabilities.
func PerformGkeVulnerabilitiesCheck(skipSecurity bool, blueprint *config.Blueprint) {
	if !skipSecurity {
		gkeVersions, err := ResolveGKEVersions(blueprint)
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
		gkeVersion = normalizeVersion(gkeVersion)
		minorVersion := semver.MajorMinor(gkeVersion)

		for _, adv := range db.Advisories {
			switch adv.Status {
			case "PENDING":
				warnings = append(warnings, fmt.Sprintf(
					"SECURITY WARNING: Your GKE version %s might be vulnerable to %s (%s). "+
						"Patches are currently PENDING in upstream GKE. See: %s",
					gkeVersion, adv.CVE, adv.Name, adv.Link))
			case "PATCHED":
				if patchedVersion, exists := adv.PatchedVersions[minorVersion]; exists {
					if compareGKEVersions(gkeVersion, patchedVersion) < 0 {
						warnings = append(warnings, fmt.Sprintf(
							"SECURITY WARNING: Your GKE version %s might be vulnerable to %s (%s). "+
								"Please upgrade your blueprint to at least %s. See: %s",
							gkeVersion, adv.CVE, adv.Name, patchedVersion, adv.Link))
					}
				}
			}
		}
	}

	uniqueWarnings := make([]string, 0, len(warnings))
	seen := make(map[string]bool)
	for _, w := range warnings {
		if !seen[w] {
			seen[w] = true
			uniqueWarnings = append(uniqueWarnings, w)
		}
	}

	return uniqueWarnings
}

const gkeClusterModule = "modules/scheduler/gke-cluster"

func hasGKECluster(bp *config.Blueprint) bool {
	hasGKE := false
	bp.WalkModulesSafe(func(_ config.ModulePath, m *config.Module) {
		if strings.Contains(m.Source, gkeClusterModule) {
			hasGKE = true
		}
	})
	return hasGKE
}

// ResolveGKEVersions determines the exact GKE versions for all GKE clusters and returns a list of resolved GKE versions used.
func ResolveGKEVersions(bp *config.Blueprint) ([]string, error) {
	if !hasGKECluster(bp) {
		return []string{}, nil
	}

	projectID := config.GetKeyFromBlueprint("project_id", *bp)
	region := config.GetKeyFromBlueprint("region", *bp)
	if projectID == "" || region == "" {
		return []string{}, fmt.Errorf("project_id and region must be defined in vars")
	}

	versions := make([]string, 0)
	var errs []string
	bp.WalkModulesSafe(func(_ config.ModulePath, m *config.Module) {
		// 1. Check for min_master_version safely
		if version := config.GetEvaluatedString("min_master_version", m, bp); version != "" {
			versions = append(versions, version)
			return // Proceed to the next module
		}

		// 2. Check for version_prefix safely
		if versionPrefix := config.GetEvaluatedString("version_prefix", m, bp); versionPrefix != "" {
			releaseChannel := config.GetEvaluatedString("release_channel", m, bp)
			latestVersion, e := fetchGKEVersionFunc(projectID, region, versionPrefix, releaseChannel)
			if e != nil {
				errs = append(errs, e.Error())
			}
			if latestVersion != "" {
				versions = append(versions, latestVersion)
			} else {
				// FALLBACK: The prefix is likely too old and no longer available in the specified channel.
				// Return the prefix itself so the vulnerability check can flag it as EOL or vulnerable.
				versions = append(versions, versionPrefix)
			}
		}
	})
	if len(errs) == 0 {
		return versions, nil
	}
	return versions, fmt.Errorf("%s", strings.Join(errs, "; "))
}

// Allow overriding the fetch function for testing ResolveGKEVersions
var fetchGKEVersionFunc = func(projectID, region, prefix, releaseChannel string) (string, error) {
	// Call the real function with default options
	return fetchLatestGKEVersionForPrefix(projectID, region, prefix, releaseChannel)
}

// Helper to format version for semver comparison
func normalizeVersion(v string) string {
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// Helper to find the highest version matching a prefix in a list
func getHighestMatchingVersion(versions []string, prefix string) string {
	var latest string
	for _, version := range versions {
		if !strings.HasPrefix(version, prefix) {
			continue
		}
		if latest == "" {
			latest = version
			continue
		}
		if compareGKEVersions(version, latest) > 0 {
			latest = version
		}
	}
	return latest
}

// fetchLatestGKEVersionForPrefix calls the GKE API to get the version a new cluster would use.
func fetchLatestGKEVersionForPrefix(projectID, region, prefix, releaseChannel string, opts ...option.ClientOption) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	service, err := container.NewService(ctx, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to create container service client: %w", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/%s", projectID, region)
	serverConfig, err := service.Projects.Locations.GetServerConfig(parent).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to get server config: %w", err)
	}

	var candidates []string
	if releaseChannel != "" && releaseChannel != "UNSPECIFIED" {
		for _, channel := range serverConfig.Channels {
			if channel.Channel == releaseChannel {
				candidates = channel.ValidVersions
				break
			}
		}
	} else {
		candidates = serverConfig.ValidMasterVersions
	}

	return getHighestMatchingVersion(candidates, prefix), nil
}

// compareGKEVersions compares two GKE versions.
// It correctly handles the -gke.X suffix by treating it as a post-release build number instead of a standard semver pre-release tag.
func compareGKEVersions(v1, v2 string) int {
	base1, build1 := parseGKEVersion(v1)
	base2, build2 := parseGKEVersion(v2)

	// Compare the base versions using standard semver comparison
	cmp := semver.Compare(base1, base2)
	if cmp != 0 {
		return cmp
	}

	// If base versions are equal, compare the GKE build numbers
	if build1 < build2 {
		return -1
	}
	if build1 > build2 {
		return 1
	}
	return 0
}

// parseGKEVersion separates the base version from the GKE build number.
func parseGKEVersion(v string) (string, int) {
	v = normalizeVersion(v)
	parts := strings.Split(v, "-gke.")
	base := parts[0]
	build := 0
	if len(parts) > 1 {
		// Ignore any trailing semver build metadata (e.g. +meta) when parsing the build number
		buildStr := strings.Split(parts[1], "+")[0]
		if b, err := strconv.Atoi(buildStr); err == nil {
			build = b
		}
	}
	return base, build
}
