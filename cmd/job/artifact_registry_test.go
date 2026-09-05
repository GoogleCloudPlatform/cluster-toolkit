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

package job

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/option"
)

func TestLookupArtifactRegistryRepos(t *testing.T) {
	// 1. Setup the httptest server with a handler to mock GCP API
	mockHandler := func(w http.ResponseWriter, r *http.Request) {
		// Verify URL matches the expected parent URL requested
		if r.URL.Path != "/v1/projects/my-project/locations/us-central1/repositories" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		pageToken := r.URL.Query().Get("pageToken")

		var resp artifactregistry.ListRepositoriesResponse

		switch pageToken {
		case "":
			// Page 1: 3 Docker repos + 1 Maven repo
			resp.Repositories = []*artifactregistry.Repository{
				{Name: "projects/my-project/locations/us-central1/repositories/repo1", Format: "DOCKER"},
				{Name: "projects/my-project/locations/us-central1/repositories/repo2", Format: "docker"},     // Case testing
				{Name: "projects/my-project/locations/us-central1/repositories/repo-maven", Format: "MAVEN"}, // Should filter out
				{Name: "projects/my-project/locations/us-central1/repositories/repo3", Format: "DOCKER"},
			}
			resp.NextPageToken = "page2"
		case "page2":
			// Page 2: 3 more Docker repos (total 6 valid ones). It should stop at 5 and hasMore=true
			resp.Repositories = []*artifactregistry.Repository{
				{Name: "projects/my-project/locations/us-central1/repositories/repo4", Format: "DOCKER"},
				{Name: "projects/my-project/locations/us-central1/repositories/repo5", Format: "DOCKER"},
				{Name: "projects/my-project/locations/us-central1/repositories/repo6", Format: "DOCKER"},
			}
			resp.NextPageToken = ""
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		bytes, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bytes)
	}

	ts := httptest.NewServer(http.HandlerFunc(mockHandler))
	defer ts.Close()

	// 2. Override newArtifactRegistryService pointing it to local server
	origNewService := newArtifactRegistryService
	defer func() { newArtifactRegistryService = origNewService }()

	newArtifactRegistryService = func(ctx context.Context) (*artifactregistry.Service, error) {
		return artifactregistry.NewService(ctx, option.WithEndpoint(ts.URL), option.WithoutAuthentication())
	}

	// 3. Call the logic
	// Testing with a zone (us-central1-c) ensures the shell.ExtractRegion resolves it to us-central1 for the URL above
	suggestions, hasMore := lookupArtifactRegistryRepos(context.Background(), "my-project", "us-central1-c")

	// 4. Assertions
	expected := []string{"repo1", "repo2", "repo3", "repo4", "repo5"}
	if !reflect.DeepEqual(suggestions, expected) {
		t.Errorf("Expected suggestions %v, got %v", expected, suggestions)
	}
	if !hasMore {
		t.Errorf("Expected hasMore to be true, got false")
	}
}
