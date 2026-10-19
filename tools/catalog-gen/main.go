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

// Binary catalog-gen generates skills/cluster-blueprint-assistant/references/catalog.yaml
// and updates the blueprint summary table in examples/README.md.
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
	flag.Parse()

	sourcereader.ModuleFS = os.DirFS(*repoRoot).(sourcereader.BaseFS)
	if err := catalog.Generate(*repoRoot, *catalogOut, *readmePath); err != nil {
		log.Fatalf("catalog-gen failed: %v", err)
	}
}
