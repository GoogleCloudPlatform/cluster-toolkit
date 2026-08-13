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

package gke

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func setupTestCustomTemplate(t *testing.T) string {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom-templates")
	err := os.MkdirAll(customPath, 0755)
	if err != nil {
		t.Fatal(err)
	}

	customContent := `custom: {{.WorkloadName}}`
	err = os.WriteFile(filepath.Join(customPath, "jobset.tmpl"), []byte(customContent), 0644)
	if err != nil {
		t.Fatal(err)
	}
	return customPath
}

func TestCustomTemplateLoading_CLIFlag(t *testing.T) {
	customPath := setupTestCustomTemplate(t)

	g := &GKEOrchestrator{
		gkeCustomTemplatesPath: customPath,
	}
	tmpl, err := g.parseGKETemplate("jobset.tmpl")
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct{ WorkloadName string }{"my-test-job"})
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	if buf.String() != "custom: my-test-job" {
		t.Errorf("Expected custom content, got %q", buf.String())
	}
}

func TestCustomTemplateLoading_RawManifest(t *testing.T) {
	customPath := setupTestCustomTemplate(t)
	g := &GKEOrchestrator{
		gkeCustomTemplatesPath: customPath,
	}

	rawContent := `apiVersion: v1
kind: Pod
metadata:
  name: static-pod`
	err := os.WriteFile(filepath.Join(customPath, "raw.tmpl"), []byte(rawContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	tmpl, err := g.parseGKETemplate("raw.tmpl")
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, nil)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	if buf.String() != rawContent {
		t.Errorf("Expected raw content, got %q", buf.String())
	}
}

func TestExtractDefaultTemplates(t *testing.T) {
	dir := t.TempDir()
	err := ExtractDefaultTemplates(dir, false)
	if err != nil {
		t.Fatalf("ExtractDefaultTemplates failed: %v", err)
	}

	// Verify that we can't overwrite without the flag
	err = ExtractDefaultTemplates(dir, false)
	if err == nil {
		t.Fatal("Expected error when overwriting without overwrite=true, got nil")
	}

	// Verify we can overwrite with the flag
	err = ExtractDefaultTemplates(dir, true)
	if err != nil {
		t.Fatalf("Expected no error when overwriting with overwrite=true, got: %v", err)
	}
}
