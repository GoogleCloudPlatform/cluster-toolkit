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
	"strings"
	"testing"
)

func TestEvaluate(t *testing.T) {
	db := &VulnerabilityDB{
		Advisories: []Advisory{
			{
				CVE:    "CVE-2026-TEST-PENDING",
				Name:   "Unpatched Vuln",
				Status: "PENDING",
				Link:   "https://example.com/pending",
			},
			{
				CVE:    "CVE-2026-TEST-PATCHED",
				Name:   "Patched Vuln",
				Status: "PATCHED",
				PatchedVersions: map[string]string{
					"v1.35": "v1.35.3-gke.1000",
					"v1.34": "1.34.7-gke.1000", // Testing handling without 'v' prefix
				},
				Link: "https://example.com/patched",
			},
		},
	}

	tests := []struct {
		name        string
		gkeVersions []string
		wantCount   int
		wantStrings []string
	}{
		{
			name:        "Vulnerable to pending only (unrelated minor version)",
			gkeVersions: []string{"v1.36.0-gke.100"},
			wantCount:   1, // Only matches PENDING
			wantStrings: []string{"CVE-2026-TEST-PENDING", "PENDING in upstream"},
		},
		{
			name:        "Vulnerable to both pending and patched (older version)",
			gkeVersions: []string{"v1.35.0-gke.500"},
			wantCount:   2,
			wantStrings: []string{
				"CVE-2026-TEST-PENDING",
				"CVE-2026-TEST-PATCHED",
				"upgrade your blueprint to at least v1.35.3-gke.1000",
			},
		},
		{
			name:        "Already patched against CVE-2026-TEST-PATCHED",
			gkeVersions: []string{"1.35.5-gke.1200"}, // Missing 'v' prefix for testing standardizer
			wantCount:   1,                           // Only PENDING matches
			wantStrings: []string{"CVE-2026-TEST-PENDING"},
		},
		{
			name:        "No matching minor version for patched vulnerability",
			gkeVersions: []string{"1.33.0-gke.100"},
			wantCount:   1, // Only PENDING matches
			wantStrings: []string{"CVE-2026-TEST-PENDING"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			warnings := evaluate(db, tc.gkeVersions)

			if len(warnings) != tc.wantCount {
				t.Errorf("evaluate() returned %d warnings; want %d", len(warnings), tc.wantCount)
			}

			for _, wantStr := range tc.wantStrings {
				found := false
				for _, w := range warnings {
					if strings.Contains(w, wantStr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("evaluate() warnings missing expected string %q\nGot warnings: %v", wantStr, warnings)
				}
			}
		})
	}
}
