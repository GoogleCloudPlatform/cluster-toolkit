// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gke

import (
	"testing"
)

func TestExtractURIPart(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		key      string
		expected string
	}{
		{
			name:     "Standard relative URI with project and region",
			uri:      "projects/my-project/regions/us-central1/resourcePolicies/my-policy",
			key:      "regions",
			expected: "us-central1",
		},
		{
			name:     "Full HTTPS URL with zone and reservation",
			uri:      "https://www.googleapis.com/compute/v1/projects/my-proj/zones/us-central1-a/reservations/my-res",
			key:      "reservations",
			expected: "my-res",
		},
		{
			name:     "Case-insensitive key matching",
			uri:      "projects/my-project/REGIONS/europe-west4/resourcePolicies/test-policy",
			key:      "regions",
			expected: "europe-west4",
		},
		{
			name:     "Trailing slash handling",
			uri:      "projects/my-proj/regions/us-central1/resourcePolicies/my-policy/",
			key:      "resourcePolicies",
			expected: "my-policy",
		},
		{
			name:     "Key at the end of URI returns empty string",
			uri:      "projects/my-proj/regions/",
			key:      "regions",
			expected: "",
		},
		{
			name:     "Key not present returns empty string",
			uri:      "projects/my-proj/zones/us-central1-a",
			key:      "regions",
			expected: "",
		},
		{
			name:     "Empty URI returns empty string",
			uri:      "",
			key:      "projects",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURIPart(tt.uri, tt.key)
			if got != tt.expected {
				t.Errorf("extractURIPart(%q, %q) = %q, want %q", tt.uri, tt.key, got, tt.expected)
			}
		})
	}
}

func TestIsPermissionDenied(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{
			name:     "403 Forbidden error string",
			errStr:   "ERROR: (gcloud.compute.reservations.describe) 403 Forbidden: Required 'compute.reservations.get' permission",
			expected: true,
		},
		{
			name:     "Permission denied error text",
			errStr:   "googleapi: Error 403: Permission denied on resource",
			expected: true,
		},
		{
			name:     "GCE required compute permission",
			errStr:   "ERROR: (gcloud.compute.reservations.describe) Could not fetch resource: - Required 'compute.reservations.get' permission for 'projects/p/zones/z/reservations/r'",
			expected: true,
		},
		{
			name:     "PERMISSION_DENIED gRPC status code",
			errStr:   "ERROR: (gcloud.compute.reservations.describe) PERMISSION_DENIED: caller does not have permission",
			expected: true,
		},
		{
			name:     "Resource name containing 403 or permission is not treated as permission denied",
			errStr:   "ERROR: (gcloud.compute.reservations.describe) The resource 'projects/p/zones/z/reservations/res-403' was not found",
			expected: false,
		},
		{
			name:     "Resource name containing permission is not treated as permission denied",
			errStr:   "ERROR: (gcloud.compute.resource-policies.describe) The resource 'projects/p/regions/r/resourcePolicies/my-permission-policy' was not found",
			expected: false,
		},
		{
			name:     "Not found error (404) returns false",
			errStr:   "ERROR: (gcloud.compute.reservations.describe) The resource was not found (404)",
			expected: false,
		},
		{
			name:     "Connection timeout returns false",
			errStr:   "i/o timeout connecting to compute.googleapis.com",
			expected: false,
		},
		{
			name:     "Empty error returns false",
			errStr:   "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPermissionDenied(tt.errStr)
			if got != tt.expected {
				t.Errorf("isPermissionDenied(%q) = %v, want %v", tt.errStr, got, tt.expected)
			}
		})
	}
}
