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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"gopkg.in/yaml.v3"
)

const (
	projectID          = "hpc-toolkit-gsc"
	releasesCollection = "release_metadata"
)

// TreeResponse represents the expected JSON structure from the GitHub Git Trees API
type TreeResponse struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
}

// MinimalBlueprint is a lightweight struct to extract only the blueprint_name
type MinimalBlueprint struct {
	BlueprintName string `yaml:"blueprint_name"`
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func main() {
	version := flag.String("version", "", "The toolkit version tag (e.g., v1.90.0)")
	flag.Parse()
	if *version == "" {
		log.Fatal("Error: -version flag is required")
	}

	// Fetch the tree once to avoid redundant API calls
	treeResp, err := fetchGitTree(*version)
	if err != nil {
		log.Fatalf("Error fetching git tree for version %s: %v", *version, err)
	}

	// Fetch required metadata to be stored in Firestore
	standardModules := fetchStandardModules(treeResp, *version)
	standardExampleFiles := fetchStandardExampleFiles(treeResp, *version)
	standardBlueprintNames := fetchStandardBlueprintNames(*version, standardExampleFiles)

	// Write to Firestore
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer client.Close()

	_, err = client.Collection(releasesCollection).Doc(*version).Set(ctx, map[string]interface{}{
		"modules":         standardModules,
		"examples":        standardExampleFiles,
		"blueprint_names": standardBlueprintNames,
	})

	if err != nil {
		log.Fatalf("Failed to write cache to Firestore: %v", err)
	}

	fmt.Printf("Successfully cached metadata for version %s in Firestore.\n", *version)
}

// fetchGitTree queries the GitHub API and decodes the JSON into a TreeResponse for the specific version.
func fetchGitTree(version string) (*TreeResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/GoogleCloudPlatform/cluster-toolkit/git/trees/%s?recursive=1", version)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from GitHub API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status: %s", resp.Status)
	}

	var treeResp TreeResponse
	if err := json.NewDecoder(resp.Body).Decode(&treeResp); err != nil {
		return nil, fmt.Errorf("failed to decode JSON response: %v", err)
	}

	return &treeResp, nil
}

func fetchStandardModules(treeResp *TreeResponse, version string) []string {
	moduleSet := make(map[string]bool)
	predefinedModules := make([]string, 0)

	// Parse the remote tree
	for _, item := range treeResp.Tree {
		// Check for Terraform and Packer files in the module directories.
		if item.Type == "blob" &&
			(strings.HasPrefix(item.Path, "modules/") || strings.HasPrefix(item.Path, "community/modules/")) &&
			(strings.HasSuffix(item.Path, ".tf") || strings.HasSuffix(item.Path, ".pkr.hcl")) {
			moduleDir := path.Dir(item.Path)
			if !moduleSet[moduleDir] {
				moduleSet[moduleDir] = true
				predefinedModules = append(predefinedModules, moduleDir)
			}
		}
	}
	if len(predefinedModules) == 0 {
		log.Printf("No modules found to cache for version %s", version)
	} else {
		fmt.Printf("Successfully fetched %d standard modules for version %s.\n", len(predefinedModules), version)
	}

	return predefinedModules
}

func fetchStandardExampleFiles(treeResp *TreeResponse, version string) []string {
	predefinedExampleFiles := make([]string, 0)

	// Parse the remote tree
	for _, item := range treeResp.Tree {
		// Check for YAML files in the example directories.
		if item.Type == "blob" &&
			(strings.HasPrefix(item.Path, "examples/") || strings.HasPrefix(item.Path, "community/examples/")) &&
			strings.HasSuffix(item.Path, ".yaml") {
			predefinedExampleFiles = append(predefinedExampleFiles, item.Path)
		}
	}

	if len(predefinedExampleFiles) == 0 {
		log.Printf("No examples found to cache for version %s", version)
	} else {
		fmt.Printf("Successfully fetched %d standard example files for version %s.\n", len(predefinedExampleFiles), version)
	}

	return predefinedExampleFiles
}

func fetchStandardBlueprintNames(version string, standardExampleFiles []string) []string {
	blueprintNamesSet := make(map[string]bool)
	blueprintNames := make([]string, 0)

	numJobs := len(standardExampleFiles)
	if numJobs == 0 {
		log.Printf("No blueprint names found to cache for version %s", version)
		return blueprintNames
	}

	jobs := make(chan string, numJobs)
	results := make(chan string, numJobs)
	var wg sync.WaitGroup

	// Set up a bounded worker pool (e.g., 10 concurrent connections)
	numWorkers := min(numJobs, 10)

	// 1. Start the worker goroutines
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go worker(w, version, jobs, results, &wg)
	}

	// 2. Feed the jobs channel with the file paths
	for _, examplePath := range standardExampleFiles {
		jobs <- examplePath
	}
	close(jobs) // Signal that no more jobs will be sent

	// 3. Wait for all workers to finish in the background, then close the results channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// 4. Collect results synchronously (prevents race conditions on the map)
	for bpName := range results {
		if !blueprintNamesSet[bpName] {
			blueprintNamesSet[bpName] = true
			blueprintNames = append(blueprintNames, bpName)
		}
	}

	if len(blueprintNames) == 0 {
		log.Printf("No blueprint names found to cache for version %s", version)
	} else {
		fmt.Printf("Successfully fetched %d standard blueprint names for version %s.\n", len(blueprintNames), version)
	}

	return blueprintNames
}

// worker fetches YAML files from GitHub and extracts the blueprint_name
func worker(id int, version string, jobs <-chan string, results chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()

	for examplePath := range jobs {
		// Create a context with a strict 10-second timeout for each file fetch
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/GoogleCloudPlatform/cluster-toolkit/%s/%s", version, examplePath)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			log.Printf("Warning [Worker %d]: failed to create request %s: %v", id, rawURL, err)
			cancel()
			continue
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Warning [Worker %d]: failed to fetch raw yaml %s: %v", id, rawURL, err)
			cancel()
			continue
		}

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err == nil {
				var bp MinimalBlueprint
				// Unmarshal gracefully ignores all fields except blueprint_name
				if err := yaml.Unmarshal(body, &bp); err == nil && bp.BlueprintName != "" {
					results <- bp.BlueprintName
				}
			}
		} else {
			log.Printf("Warning [Worker %d]: received status %d when fetching %s", id, resp.StatusCode, rawURL)
		}

		resp.Body.Close()
		cancel() // Release the context resources
	}
}
