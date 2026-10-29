// Copyright 2026 Google LLC
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

// Binary catalog-gen generates skills/cluster-blueprint-assistant/references/catalog.yaml,
// updates the blueprint summary tables in examples/README.md and
// community/examples/README.md, and generates root llms.txt and llms-full.txt.
package main

import (
	"flag"
	"hpc-toolkit/pkg/catalog"
	"hpc-toolkit/pkg/sourcereader"
	"log"
	"os"
)

func main() {
	repoRoot := flag.String("root", ".", "Path to Cluster Toolkit repository root")
	catalogOut := flag.String("catalog-out", "skills/cluster-blueprint-assistant/references/catalog.yaml", "Output path for catalog.yaml")
	readmePath := flag.String("readme", "examples/README.md", "Path to examples/README.md")
	communityReadme := flag.String("community-readme", "community/examples/README.md", "Path to community/examples/README.md")
	llmsTxt := flag.String("llms-out", "llms.txt", "Output path for llms.txt")
	llmsFullTxt := flag.String("llms-full-out", "llms-full.txt", "Output path for llms-full.txt")
	flag.Parse()

	sourcereader.ModuleFS = os.DirFS(*repoRoot).(sourcereader.BaseFS)
	err := catalog.Generate(*repoRoot, catalog.OutputPaths{
		CatalogOut:      *catalogOut,
		ExamplesReadme:  *readmePath,
		CommunityReadme: *communityReadme,
		LLMsTxtPath:     *llmsTxt,
		LLMsFullTxtPath: *llmsFullTxt,
	})
	if err != nil {
		log.Fatalf("catalog-gen failed: %v", err)
	}
}
