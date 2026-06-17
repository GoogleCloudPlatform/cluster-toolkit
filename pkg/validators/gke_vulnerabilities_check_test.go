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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hpc-toolkit/pkg/config"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"
	"google.golang.org/api/option"
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

func TestResolveGKEVersions(t *testing.T) {
	// Temporarily override the function pointer for testing
	originalFetchFunc := fetchGKEVersionFunc
	fetchGKEVersionFunc = mockFetchGKEVersionForPrefix
	// Restore original function after test
	defer func() { fetchGKEVersionFunc = originalFetchFunc }()

	testCases := []struct {
		name      string
		blueprint config.Blueprint
		expected  []string
		hasError  bool
	}{
		{
			name: "simple min_master_version",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal("test-project"),
					"region":     cty.StringVal("us-central1"),
				}),
				Groups: []config.Group{
					{
						Name: "primary",
						Modules: []config.Module{
							{
								ID:     "gke",
								Source: "modules/scheduler/gke-cluster",
								Settings: config.NewDict(map[string]cty.Value{
									"min_master_version": cty.StringVal("1.37.0"),
								}),
							},
						},
					},
				},
			},
			expected: []string{"1.37.0"},
		},
		{
			name: "version_prefix rapid",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal("test-project"),
					"region":     cty.StringVal("us-central1"),
				}),
				Groups: []config.Group{
					{
						Name: "primary",
						Modules: []config.Module{
							{
								ID:     "gke",
								Source: "modules/scheduler/gke-cluster",
								Settings: config.NewDict(map[string]cty.Value{
									"version_prefix":  cty.StringVal("1.36."),
									"release_channel": cty.StringVal("RAPID"),
								}),
							},
						},
					},
				},
			},
			expected: []string{"1.36.1-gke.100"},
		},
		{
			name: "version_prefix regular default",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal("test-project"),
					"region":     cty.StringVal("us-central1"),
				}),
				Groups: []config.Group{
					{
						Name: "primary",
						Modules: []config.Module{
							{
								ID:     "gke",
								Source: "modules/scheduler/gke-cluster",
								Settings: config.NewDict(map[string]cty.Value{
									"version_prefix":  cty.StringVal("1.35."),
									"release_channel": cty.StringVal("REGULAR"),
								}),
							},
						},
					},
				},
			},
			expected: []string{"1.35.2-gke.200"},
		},
		{
			name: "version_prefix fallback",
			blueprint: config.Blueprint{
				Vars: config.NewDict(map[string]cty.Value{
					"project_id": cty.StringVal("test-project"),
					"region":     cty.StringVal("us-central1"),
				}),
				Groups: []config.Group{
					{
						Name: "primary",
						Modules: []config.Module{
							{
								ID:     "gke",
								Source: "modules/scheduler/gke-cluster",
								Settings: config.NewDict(map[string]cty.Value{
									"version_prefix": cty.StringVal("1.32."), // Non-existent in mock
								}),
							},
						},
					},
				},
			},
			expected: []string{"1.32."},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			versions, err := ResolveGKEVersions(&tc.blueprint)
			if (err != nil) != tc.hasError {
				t.Errorf("ResolveGKEVersions() error = %v, wantErr %v", err, tc.hasError)
				return
			}

			if diff := cmp.Diff(tc.expected, versions); diff != "" {
				t.Errorf("ResolveGKEVersions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFetchLatestGKEVersionForPrefix(t *testing.T) {
	// 1. Create a mock HTTP server simulating the GCP Container API Response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the API URL path is correctly formatted
		expectedPath := "/v1/projects/test-project/locations/us-central1/serverConfig"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %q, got %q", expectedPath, r.URL.Path)
		}

		// Provide a mock JSON response matching container.ServerConfig
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"channels": [
				{
					"channel": "RAPID",
					"validVersions": ["1.35.3-gke.1943000", "1.36.0-gke.1555000"]
				},
				{
					"channel": "REGULAR",
					"validVersions": ["1.34.7-gke.1292000", "1.35.1-gke.1000"]
				}
			]
		}`)
	}))
	defer mockServer.Close()

	// 2. Pass options to route traffic to the mock server and disable auth
	clientOpt := option.WithEndpoint(mockServer.URL)
	noAuthOpt := option.WithoutAuthentication()

	tests := []struct {
		name        string
		prefix      string
		channel     string
		wantVersion string
	}{
		{
			name:        "Highest version across all channels for 1.35 prefix",
			prefix:      "1.35.",
			channel:     "RAPID",
			wantVersion: "1.35.3-gke.1943000", // Matches RAPID, which is higher than REGULAR's 1.35.1
		},
		{
			name:        "Highest version for 1.34 prefix",
			prefix:      "1.34.",
			channel:     "REGULAR",
			wantVersion: "1.34.7-gke.1292000",
		},
		{
			name:        "Fallback to empty if no matching prefix is found",
			prefix:      "1.29.",
			channel:     "UNSPECIFIED",
			wantVersion: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := fetchLatestGKEVersionForPrefix("test-project", "us-central1", tc.prefix, tc.channel, clientOpt, noAuthOpt)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if got != tc.wantVersion {
				t.Errorf("fetchLatestGKEVersionForPrefix() = %v, want %v", got, tc.wantVersion)
			}
		})
	}
}

func mockFetchGKEVersionForPrefix(projectID, region, prefix, releaseChannel string) (string, error) {
	// Mock responses based on channel and prefix
	if projectID == "test-project" && region == "us-central1" {
		if releaseChannel == "RAPID" && strings.HasPrefix("1.36.", prefix) {
			return "1.36.1-gke.100", nil
		}
		if releaseChannel == "REGULAR" && strings.HasPrefix("1.35.", prefix) {
			return "1.35.2-gke.200", nil
		}
		if prefix == "1.34." {
			return "1.34.9-gke.100", nil // Fallback test
		}
	}
	return "", nil
}
