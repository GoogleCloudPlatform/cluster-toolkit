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
	"fmt"
	"strings"
	"time"

	"hpc-toolkit/pkg/shell"

	"google.golang.org/api/artifactregistry/v1"
)

// lookupArtifactRegistryRepos queries Artifact Registry to find up to 5 Docker repositories
// in the specified project and region.
var lookupArtifactRegistryRepos = func(ctx context.Context, projectID, location string) []string {
	if projectID == "" || location == "" {
		return nil
	}

	region := shell.ExtractRegion(location)

	service, err := artifactregistry.NewService(ctx)
	if err != nil {
		return nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	parent := fmt.Sprintf("projects/%s/locations/%s", projectID, region)
	maxSuggestions := 5
	var suggestions []string
	hasMore := false
	nextPageToken := ""

	for {
		req := service.Projects.Locations.Repositories.List(parent)
		if nextPageToken != "" {
			req.PageToken(nextPageToken)
		}

		resp, err := req.Context(timeoutCtx).Do()
		if err != nil {
			break
		}

		for _, repo := range resp.Repositories {
			if repo.Format != "DOCKER" {
				continue
			}
			repoParts := strings.Split(repo.Name, "/")
			repoID := repoParts[len(repoParts)-1]

			if len(suggestions) < maxSuggestions {
				suggestions = append(suggestions, repoID)
			} else {
				hasMore = true
				break
			}
		}

		if hasMore || resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	if hasMore {
		suggestions = append(suggestions, "... (and more)")
	}

	return suggestions
}
