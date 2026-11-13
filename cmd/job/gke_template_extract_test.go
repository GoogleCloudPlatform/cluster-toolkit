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

package job

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGKETemplateExtractCmd_ExtractsAndRefusesOverwrite(t *testing.T) {
	outputDir = filepath.Join(t.TempDir(), "gke-templates")
	overwriteDst = false
	t.Cleanup(func() { outputDir, overwriteDst = "./gke-templates", false })

	if err := gkeTemplateExtractCmd.RunE(gkeTemplateExtractCmd, nil); err != nil {
		t.Fatalf("gke-template-extract failed: %v", err)
	}
	for _, name := range []string{"base.tmpl", "jobset.tmpl", "pathways_jobset.tmpl", "job.tmpl"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Errorf("expected %s to be extracted: %v", name, err)
		}
	}
	err := gkeTemplateExtractCmd.RunE(gkeTemplateExtractCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("re-extracting without --overwrite: error = %v, want error naming --overwrite", err)
	}
	overwriteDst = true
	if err := gkeTemplateExtractCmd.RunE(gkeTemplateExtractCmd, nil); err != nil {
		t.Fatalf("gke-template-extract --overwrite failed: %v", err)
	}
}
