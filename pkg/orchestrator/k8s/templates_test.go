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

package k8s

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/safetext/yamltemplate"
)

func setupTestCustomTemplateDir(t *testing.T) string {
	t.Helper()
	customPath := filepath.Join(t.TempDir(), "custom-templates")
	if err := os.MkdirAll(customPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customPath, "jobset.tmpl"), []byte(`custom: {{.WorkloadName}}`), 0644); err != nil {
		t.Fatal(err)
	}
	return customPath
}

func TestParseYAMLTemplate_CustomAndRaw(t *testing.T) {
	t.Parallel()
	customPath := setupTestCustomTemplateDir(t)
	tmpl, err := ParseYAMLTemplate(customPath, "jobset.tmpl")
	if err != nil {
		t.Fatalf("ParseYAMLTemplate failed: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ WorkloadName string }{"my-test-job"}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if buf.String() != "custom: my-test-job" {
		t.Errorf("got %q, want %q", buf.String(), "custom: my-test-job")
	}

	rawContent := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: static-pod"
	if err := os.WriteFile(filepath.Join(customPath, "raw.tmpl"), []byte(rawContent), 0644); err != nil {
		t.Fatal(err)
	}
	rawTmpl, err := ParseYAMLTemplate(customPath, "raw.tmpl")
	if err != nil {
		t.Fatalf("ParseYAMLTemplate(raw.tmpl) failed: %v", err)
	}
	buf.Reset()
	if err := rawTmpl.Execute(&buf, nil); err != nil || buf.String() != rawContent {
		t.Errorf("got (%q, %v), want (%q, nil)", buf.String(), err, rawContent)
	}
}

func TestExtractDefaultTemplates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := ExtractDefaultTemplates(dir, false); err != nil {
		t.Fatalf("ExtractDefaultTemplates failed: %v", err)
	}
	wantFiles := []string{
		"base.tmpl",
		"filestore.tmpl",
		"gcs_fuse_pv_pvc.tmpl",
		"job.tmpl",
		"jobset.tmpl",
		"local_queue.tmpl",
		"pathways_jobset.tmpl",
		"priority_classes.tmpl",
	}
	for _, wantFile := range wantFiles {
		if _, err := os.Stat(filepath.Join(dir, wantFile)); err != nil {
			t.Errorf("expected extracted template %q to exist: %v", wantFile, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(wantFiles) {
		t.Errorf("extracted %d files, want %d", len(entries), len(wantFiles))
	}
	if err := ExtractDefaultTemplates(dir, false); err == nil {
		t.Fatal("expected error when overwriting without overwrite=true")
	}
	if err := ExtractDefaultTemplates(dir, true); err != nil {
		t.Fatalf("expected overwrite=true to succeed, got: %v", err)
	}
}

// loadBlocks returns the base.tmpl blocks LoadWorkloadTemplate compiles for customDir.
func loadBlocks(t *testing.T, customDir string) map[string]*yamltemplate.Template {
	t.Helper()
	_, blocks, err := LoadWorkloadTemplate(customDir, "jobset.tmpl")
	if err != nil {
		t.Fatalf("LoadWorkloadTemplate(%q) failed: %v", customDir, err)
	}
	return blocks
}

func TestLoadWorkloadTemplate_DirectoryErrors(t *testing.T) {
	t.Parallel()
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")
	if _, _, err := LoadWorkloadTemplate(missingDir, "jobset.tmpl"); err == nil || !strings.Contains(err.Error(), "--gke-custom-templates-path") {
		t.Errorf("expected missing dir error naming the flag, got: %v", err)
	}
	if err := ValidateCustomWorkloadTemplates(missingDir, "jobset.tmpl"); err == nil {
		t.Errorf("expected ValidateCustomWorkloadTemplates to fail on missing dir")
	}

	fileNotDir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileNotDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseYAMLTemplate(fileNotDir, "jobset.tmpl"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected not-a-directory error, got: %v", err)
	}

}

func TestLoadWorkloadTemplate_EmbeddedTemplates(t *testing.T) {
	t.Parallel()
	blocks := loadBlocks(t, "")
	if blocks["pod_template"] == nil || blocks["workload_labels"] == nil || blocks["telemetry_sidecar"] == nil || blocks["base.tmpl"] != nil {
		t.Fatalf("expected embedded base blocks without the root, got %v", slices.Sorted(maps.Keys(blocks)))
	}
	for _, name := range []string{"job.tmpl", "pathways_jobset.tmpl"} {
		if _, _, err := LoadWorkloadTemplate("", name); err != nil {
			t.Errorf("embedded %s failed to load: %v", name, err)
		}
	}
	if _, _, err := LoadWorkloadTemplate("", "missing.tmpl"); err == nil || !strings.Contains(err.Error(), "missing.tmpl") {
		t.Errorf("unknown embedded workload template: error = %v, want parse error naming it", err)
	}
}

func TestReindentYAML(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		indent int
		in     string
		want   string
	}{
		{name: "empty", indent: 4, in: "  \n\t\n ", want: ""},
		{name: "strips common prefix and preserves blank lines", indent: 2, in: "\n    a: 1\n\n      b: 2\n", want: "  a: 1\n\n    b: 2"},
		{name: "zero indent", indent: 0, in: "  key: val", want: "key: val"},
		{name: "deeper first line keeps relative indentation", indent: 2, in: "      - a\n    b: 1", want: "    - a\n  b: 1"},
		{name: "tab-indented lines count as column zero", indent: 2, in: "\tkey: val\n  other: 1", want: "  \tkey: val\n    other: 1"},
	}
	for _, tc := range cases {
		if got := ReindentYAML(tc.indent, tc.in); got != tc.want {
			t.Errorf("%s: ReindentYAML(%d, %q) = %q, want %q", tc.name, tc.indent, tc.in, got, tc.want)
		}
	}
}

type baseBlockTestData struct {
	WorkloadName         string
	KueueQueueName       string
	PriorityClassName    string
	ServiceAccountName   string
	ImagePullSecrets     string
	MLDiagnosticsEnabled bool
	// TelemetryCollectorImage is read by workload_labels to decide between the GKE webhook label and the native sidecar.
	TelemetryCollectorImage string
}

func executeBaseBlock(t *testing.T, blocks map[string]*yamltemplate.Template, name string, data any) string {
	t.Helper()
	bt, ok := blocks[name]
	if !ok {
		t.Fatalf("block %q not compiled; have %v", name, slices.Sorted(maps.Keys(blocks)))
	}
	var buf bytes.Buffer
	if err := bt.Execute(&buf, data); err != nil {
		t.Fatalf("executing block %q: %v", name, err)
	}
	return buf.String()
}

func baseBlockFixture() baseBlockTestData {
	return baseBlockTestData{
		WorkloadName: "job-a", KueueQueueName: "q1", PriorityClassName: "high",
		ServiceAccountName: "ksa", ImagePullSecrets: "- name: reg-sec", MLDiagnosticsEnabled: true,
	}
}

func writeCustomBase(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "base.tmpl"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// Every block is compiled as its own yamltemplate root on purpose: safetext binds the StructuralData
// callback to the UUID minted by New(), and Clone()/Lookup() mint a fresh UUID without rebinding it, so a
// cloned block fails every execution with "Invalid YAML Template". This test pins the working behaviour.
func TestLoadWorkloadTemplate_ExecutesEmbeddedBlocksWithStructuralData(t *testing.T) {
	t.Parallel()
	data := baseBlockFixture()
	blocks := loadBlocks(t, "")
	labels := executeBaseBlock(t, blocks, "workload_labels", data)
	for _, want := range []string{"gcluster.google.com/workload: job-a", "kueue.x-k8s.io/queue-name: q1", `managed-mldiagnostics-gke: "true"`} {
		if !strings.Contains(labels, want) {
			t.Errorf("workload_labels missing %q:\n%s", want, labels)
		}
	}
	auth := executeBaseBlock(t, blocks, "pod_auth_and_priority", data)
	for _, want := range []string{"priorityClassName: high", "serviceAccountName: ksa", "imagePullSecrets:\n  - name: reg-sec"} {
		if !strings.Contains(auth, want) {
			t.Errorf("pod_auth_and_priority missing %q:\n%s", want, auth)
		}
	}
	injected := data
	injected.PriorityClassName = "high\n  hostPID: true"
	if err := blocks["pod_auth_and_priority"].Execute(io.Discard, injected); !errors.Is(err, yamltemplate.ErrYAMLInjection) {
		t.Errorf("newline in PriorityClassName: error = %v, want ErrYAMLInjection", err)
	}
}

func TestLoadWorkloadTemplate_CustomOverrideAddsAndReplacesBlocks(t *testing.T) {
	t.Parallel()
	data := baseBlockFixture()
	customDir := t.TempDir()
	writeCustomBase(t, customDir, "{{- define \"workload_labels\" -}}\nteam: platform\n{{- end -}}\n{{- define \"extra_block\" -}}\nextra: {{.WorkloadName}}\n{{- end -}}\n")
	custom := loadBlocks(t, customDir)
	if got := executeBaseBlock(t, custom, "workload_labels", data); got != "team: platform" {
		t.Errorf("overridden workload_labels = %q, want %q", got, "team: platform")
	}
	if got := executeBaseBlock(t, custom, "pod_auth_and_priority", data); !strings.Contains(got, "serviceAccountName: ksa") {
		t.Errorf("non-overridden block lost its embedded body:\n%s", got)
	}
	if got := executeBaseBlock(t, custom, "extra_block", data); got != "extra: job-a" {
		t.Errorf("custom-only block = %q, want %q", got, "extra: job-a")
	}
}

func TestLoadWorkloadTemplate_EmptyOverrideKeepsEmbeddedAndMalformedFails(t *testing.T) {
	t.Parallel()
	data := baseBlockFixture()
	customDir := t.TempDir()
	writeCustomBase(t, customDir, "{{- define \"workload_labels\" -}}{{/* intentionally empty */}}{{- end -}}\n")
	kept := loadBlocks(t, customDir)
	if got := executeBaseBlock(t, kept, "workload_labels", data); !strings.Contains(got, "gcluster.google.com/workload: job-a") {
		t.Errorf("empty override must keep the embedded body, got %q", got)
	}
	writeCustomBase(t, customDir, "{{- define \"pod_auth_and_priority\" -}}{{ \"\" }}{{- end -}}\n")
	if got := executeBaseBlock(t, loadBlocks(t, customDir), "pod_auth_and_priority", data); got != "" {
		t.Errorf("empty-string action override = %q, want empty string", got)
	}
	writeCustomBase(t, customDir, "{{ define \"workload_labels\" }}{{ .Unclosed")
	if _, _, err := LoadWorkloadTemplate(customDir, "jobset.tmpl"); err == nil || !strings.Contains(err.Error(), "custom base.tmpl") {
		t.Errorf("malformed custom base.tmpl: error = %v, want parse error naming custom base.tmpl", err)
	}
	// A CRLF override (e.g. edited on Windows) renders with LF line endings only.
	writeCustomBase(t, customDir, "{{- define \"workload_labels\" -}}\r\nteam: crlf\r\n{{- end -}}\r\n")
	if got := executeBaseBlock(t, loadBlocks(t, customDir), "workload_labels", data); got != "team: crlf" {
		t.Errorf("CRLF override rendered %q, want %q", got, "team: crlf")
	}
}

func TestParseTextTemplate_CustomEmbeddedAndUnreadable(t *testing.T) {
	t.Parallel()
	if _, err := ParseTextTemplate("", "filestore.tmpl"); err != nil {
		t.Fatalf("embedded filestore.tmpl should parse: %v", err)
	}
	if _, err := ParseTextTemplate("", "does-not-exist.tmpl"); err == nil {
		t.Error("expected an error for an unknown embedded template")
	}
	customDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(customDir, "filestore.tmpl"), []byte("custom: {{.Name}}"), 0644); err != nil {
		t.Fatal(err)
	}
	tmpl, err := ParseTextTemplate(customDir, "filestore.tmpl")
	if err != nil {
		t.Fatalf("custom filestore.tmpl should parse: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ Name string }{"fs"}); err != nil || buf.String() != "custom: fs" {
		t.Errorf("custom text template = (%q, %v), want (%q, nil)", buf.String(), err, "custom: fs")
	}
	// An unreadable custom template (here: a directory with the template's name) must surface, not fall back.
	if err := os.MkdirAll(filepath.Join(customDir, "jobset.tmpl"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseYAMLTemplate(customDir, "jobset.tmpl"); err == nil || !strings.Contains(err.Error(), "failed to access custom template") {
		t.Errorf("unreadable custom template: error = %v, want access error", err)
	}
	if err := os.MkdirAll(filepath.Join(customDir, "base.tmpl"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadWorkloadTemplate(customDir, "job.tmpl"); err == nil || !strings.Contains(err.Error(), "base.tmpl") {
		t.Errorf("unreadable custom base.tmpl: error = %v, want access error naming base.tmpl", err)
	}
}

func TestValidateCustomWorkloadTemplatesAndLegacyWarning(t *testing.T) {
	t.Parallel()
	if err := ValidateCustomWorkloadTemplates("", "pathways_jobset.tmpl"); err != nil {
		t.Errorf("empty custom dir must validate: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.tmpl"), []byte("{{- define \"workload_labels\" -}}\nteam: platform\n{{- end -}}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pathways_jobset.tmpl"), []byte("kind: JobSet\nname: {{.WorkloadName}}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, tmplName := range []string{"pathways_jobset.tmpl", "jobset.tmpl", "job.tmpl"} {
		if err := ValidateCustomWorkloadTemplates(dir, tmplName); err != nil {
			t.Errorf("ValidateCustomWorkloadTemplates(%q) = %v, want nil", tmplName, err)
		}
	}
	// Only the active workload template is parsed: a broken jobset.tmpl fails JobSet submissions but not Pathways ones.
	if err := os.WriteFile(filepath.Join(dir, "jobset.tmpl"), []byte("{{ .Unclosed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCustomWorkloadTemplates(dir, "jobset.tmpl"); err == nil || !strings.Contains(err.Error(), "unclosed action") {
		t.Errorf("malformed jobset.tmpl: error = %v, want unclosed action", err)
	}
	if err := ValidateCustomWorkloadTemplates(dir, "pathways_jobset.tmpl"); err != nil {
		t.Errorf("Pathways validation must not parse jobset.tmpl: %v", err)
	}
}

func TestValidateCustomWorkloadTemplates_LegacyAndAuxiliaryTemplates(t *testing.T) {
	t.Parallel()
	// A pre-refactor template (no .RenderBlock) warns whether or not base.tmpl is present, and still loads.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.tmpl"), []byte("{{- define \"workload_labels\" -}}\nteam: platform\n{{- end -}}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pathways_jobset.tmpl"), []byte("kind: JobSet\nname: {{.WorkloadName}}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCustomWorkloadTemplates(dir, "pathways_jobset.tmpl"); err != nil {
		t.Errorf("legacy pathways_jobset.tmpl must still validate: %v", err)
	}
	noBase := t.TempDir()
	if err := os.WriteFile(filepath.Join(noBase, "jobset.tmpl"), []byte("kind: JobSet"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCustomWorkloadTemplates(noBase, "jobset.tmpl"); err != nil {
		t.Errorf("custom jobset.tmpl without base.tmpl must validate: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "jobset.tmpl"), []byte("labels:\n{{(StructuralData (.RenderBlock \"workload_labels\" 2))}}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCustomWorkloadTemplates(dir, "jobset.tmpl"); err != nil {
		t.Errorf("custom jobset.tmpl using .RenderBlock must validate: %v", err)
	}

	// Present auxiliary templates (yamltemplate and text/template) are also validated upfront.
	for _, auxName := range []string{"local_queue.tmpl", "priority_classes.tmpl", "filestore.tmpl", "gcs_fuse_pv_pvc.tmpl"} {
		auxDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(auxDir, auxName), []byte("{{ .Broken"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ValidateCustomWorkloadTemplates(auxDir, "jobset.tmpl"); err == nil {
			t.Errorf("expected ValidateCustomWorkloadTemplates to fail on malformed custom %s", auxName)
		}
	}
}

func TestExtractDefaultTemplates_UnwritableTarget(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ExtractDefaultTemplates(filepath.Join(file, "sub"), true); err == nil || !strings.Contains(err.Error(), "failed to create directory") {
		t.Errorf("ExtractDefaultTemplates under a file: error = %v, want create-directory error", err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "base.tmpl"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ExtractDefaultTemplates(dir, true); err == nil || !strings.Contains(err.Error(), "failed to write file") {
		t.Errorf("ExtractDefaultTemplates with directory at file path: error = %v, want write error", err)
	}
}

// captureTemplateLogs redirects logInfo/logWarn for one test and returns the recorded messages.
func captureTemplateLogs(t *testing.T) (infos, warns *[]string) {
	t.Helper()
	var i, w []string
	origInfo, origWarn := logInfo, logWarn
	logInfo = func(f string, a ...any) { i = append(i, fmt.Sprintf(f, a...)) }
	logWarn = func(f string, a ...any) { w = append(w, fmt.Sprintf(f, a...)) }
	t.Cleanup(func() { logInfo, logWarn = origInfo, origWarn })
	return &i, &w
}

// TestCustomTemplateLogs_PreflightQuietRenderOnce pins that a submit (preflight followed by the render-pass
// load) reports each custom template and the legacy warning exactly once.
func TestCustomTemplateLogs_PreflightQuietRenderOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "jobset.tmpl"), []byte("kind: JobSet\nname: {{.WorkloadName}}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	infos, warns := captureTemplateLogs(t)

	if err := ValidateCustomWorkloadTemplates(dir, "jobset.tmpl"); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if len(*infos)+len(*warns) != 0 {
		t.Fatalf("preflight must load quietly, logged infos=%q warns=%q", *infos, *warns)
	}
	if _, _, err := LoadWorkloadTemplate(dir, "jobset.tmpl"); err != nil {
		t.Fatalf("render-pass load failed: %v", err)
	}
	wantInfo := 0
	for _, m := range *infos {
		if strings.Contains(m, `Loading GKE template "jobset.tmpl"`) {
			wantInfo++
		}
	}
	if wantInfo != 1 || len(*warns) != 1 || !strings.Contains((*warns)[0], "Legacy custom template") {
		t.Errorf("after preflight+render want 1 load message and 1 legacy warning, got infos=%q warns=%q", *infos, *warns)
	}
}

// TestCustomTemplateLoading_EmptyFileAndWarningOrder pins two edge cases: an empty custom workload template is
// an error rather than a silent fallback (an empty custom base.tmpl stays a valid "no overrides" file), and a
// legacy template with a syntax error fails without first emitting the legacy warning.
func TestCustomTemplateLoading_EmptyFileAndWarningOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "jobset.tmpl"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadWorkloadTemplate(dir, "jobset.tmpl"); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("empty custom jobset.tmpl: error = %v, want 'is empty'", err)
	}
	if _, err := ParseYAMLTemplate(dir, "jobset.tmpl"); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("empty custom template via ParseYAMLTemplate: error = %v, want 'is empty'", err)
	}

	baseOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseOnly, "base.tmpl"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, blocks, err := LoadWorkloadTemplate(baseOnly, "jobset.tmpl"); err != nil || blocks["pod_template"] == nil {
		t.Errorf("empty custom base.tmpl must fall back to embedded blocks: err=%v blocks=%d", err, len(blocks))
	}

	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, "jobset.tmpl"), []byte("kind: JobSet\n{{ .Unclosed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, warns := captureTemplateLogs(t)
	if _, _, err := LoadWorkloadTemplate(broken, "jobset.tmpl"); err == nil || !strings.Contains(err.Error(), "unclosed action") {
		t.Fatalf("malformed legacy template: error = %v, want unclosed action", err)
	}
	if len(*warns) != 0 {
		t.Errorf("a template that fails to parse must not emit the legacy warning first, got %q", *warns)
	}
}
