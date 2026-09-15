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
	"path"
	"strings"
	"time"

	"hpc-toolkit/pkg/shell"

	"google.golang.org/api/artifactregistry/v1"
)

var newArtifactRegistryService = func(ctx context.Context) (*artifactregistry.Service, error) {
	return artifactregistry.NewService(ctx)
}

func isDockerRepo(repo *artifactregistry.Repository) bool {
	return repo != nil && strings.EqualFold(repo.Format, "DOCKER")
}

func collectDockerRepos(repos []*artifactregistry.Repository, suggestions []string, max int) ([]string, bool) {
	for _, repo := range repos {
		if !isDockerRepo(repo) {
			continue
		}
		if len(suggestions) < max {
			suggestions = append(suggestions, path.Base(repo.Name))
		} else {
			return suggestions, true
		}
	}
	return suggestions, false
}

// lookupArtifactRegistryRepos queries Artifact Registry to find up to 5 Docker repositories
// in the specified project and region.
var lookupArtifactRegistryRepos = func(ctx context.Context, projectID, location string) ([]string, bool) {
	if projectID == "" || location == "" {
		return nil, false
	}

	region := shell.ExtractRegion(location)

	service, err := newArtifactRegistryService(ctx)
	if err != nil {
		return nil, false
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
		if err != nil || resp == nil {
			break
		}

		suggestions, hasMore = collectDockerRepos(resp.Repositories, suggestions, maxSuggestions)
		if hasMore || resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	return suggestions, hasMore
}
