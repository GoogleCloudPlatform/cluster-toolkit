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
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"hpc-toolkit/pkg/logging"

	"github.com/google/safetext/yamltemplate"
)

//go:embed templates/*
var templatesFS embed.FS

func templateFuncs() yamltemplate.FuncMap {
	return yamltemplate.FuncMap{
		"reindent": ReindentYAML,
	}
}

// ReindentYAML strips the common leading indentation of s and indents its non-blank lines by indent spaces.
func ReindentYAML(indent int, s string) string {
	lines := strings.Split(strings.Trim(s, "\n"), "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if lead := len(line) - len(strings.TrimLeft(line, " ")); minIndent < 0 || lead < minIndent {
			minIndent = lead
		}
	}
	if minIndent < 0 {
		return ""
	}
	padding := strings.Repeat(" ", indent)
	out := make([]string, len(lines))
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			out[i] = padding + line[minIndent:]
		}
	}
	return strings.Join(out, "\n")
}

// readCustomTemplateBytes returns the contents of name in customDir, or nil when customDir is unset or
// does not contain name.
func readCustomTemplateBytes(customDir, name string) ([]byte, error) {
	if customDir == "" {
		return nil, nil
	}
	stat, err := os.Stat(customDir)
	if err != nil {
		return nil, fmt.Errorf("custom template directory %q (--gke-custom-templates-path) could not be accessed: %w", customDir, err)
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("custom template path %q (--gke-custom-templates-path) is not a directory", customDir)
	}
	localFile := filepath.Join(customDir, name)
	data, err := os.ReadFile(localFile)
	switch {
	case os.IsNotExist(err):
		logging.Info("Template %q not found in custom path %q. Falling back to embedded default.", name, customDir)
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("failed to access custom template %q: %w", localFile, err)
	}
	logging.Info("Loading GKE template %q from custom path %q", name, customDir)
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), nil
}

// readTemplateBytes returns the custom override of name when present (reporting isCustom), otherwise the
// embedded default.
func readTemplateBytes(customDir, name string) (data []byte, isCustom bool, err error) {
	if data, err = readCustomTemplateBytes(customDir, name); err != nil || data != nil {
		return data, data != nil, err
	}
	data, err = templatesFS.ReadFile("templates/" + name)
	return data, false, err
}

// ParseYAMLTemplate loads a safetext YAML template from customDir (if present) or the embedded defaults.
func ParseYAMLTemplate(customDir, name string) (*yamltemplate.Template, error) {
	data, _, err := readTemplateBytes(customDir, name)
	if err != nil {
		return nil, err
	}
	return yamltemplate.New(name).Funcs(templateFuncs()).Parse(string(data))
}

// ParseTextTemplate loads a standard Go text/template from customDir (if present) or the embedded defaults.
func ParseTextTemplate(customDir, name string) (*template.Template, error) {
	data, _, err := readTemplateBytes(customDir, name)
	if err != nil {
		return nil, err
	}
	return template.New(name).Parse(string(data))
}

// LoadWorkloadTemplate parses the workload template name together with the base.tmpl blocks it renders via
// .RenderBlock, each overlaid with any override found in customDir.
func LoadWorkloadTemplate(customDir, name string) (*yamltemplate.Template, map[string]*yamltemplate.Template, error) {
	data, isCustom, err := readTemplateBytes(customDir, name)
	if err != nil {
		return nil, nil, err
	}
	customBase, err := readCustomTemplateBytes(customDir, "base.tmpl")
	if err != nil {
		return nil, nil, err
	}
	if isCustom && !bytes.Contains(data, []byte(".RenderBlock")) {
		logging.Warn("Legacy custom template %q in %q does not reference .RenderBlock; features and overrides delivered via base.tmpl (e.g. sidecars) will not apply. Extract fresh defaults with 'gcluster job gke-template-extract --output-dir=<new-dir>' and merge your changes.", name, customDir)
	}
	tmpl, err := yamltemplate.New(name).Funcs(templateFuncs()).Parse(string(data))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse %s template: %w", name, err)
	}
	blocks, err := loadBaseBlocks(customBase)
	if err != nil {
		return nil, nil, err
	}
	return tmpl, blocks, nil
}

// ValidateCustomWorkloadTemplates parses base.tmpl, the active workloadTemplate, and any optional auxiliary
// templates (filestore.tmpl, gcs_fuse_pv_pvc.tmpl, local_queue.tmpl, priority_classes.tmpl) present in
// customDir upfront, so template syntax mistakes surface before any image build or cluster call.
// This check is parse-time only; unknown template fields or .RenderBlock names fail when the template is rendered.
func ValidateCustomWorkloadTemplates(customDir, workloadTemplate string) error {
	if customDir == "" {
		return nil
	}
	if _, _, err := LoadWorkloadTemplate(customDir, workloadTemplate); err != nil {
		return err
	}
	for _, name := range []string{"local_queue.tmpl", "priority_classes.tmpl"} {
		if err := validateOptionalCustomTemplate(customDir, name, func() error {
			_, err := ParseYAMLTemplate(customDir, name)
			return err
		}); err != nil {
			return err
		}
	}
	for _, name := range []string{"filestore.tmpl", "gcs_fuse_pv_pvc.tmpl"} {
		if err := validateOptionalCustomTemplate(customDir, name, func() error {
			_, err := ParseTextTemplate(customDir, name)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalCustomTemplate(customDir, name string, parse func() error) error {
	if _, err := os.Stat(filepath.Join(customDir, name)); os.IsNotExist(err) {
		return nil
	}
	return parse()
}

// loadBaseBlocks compiles every named block defined in the embedded base.tmpl or in customBase, with the
// custom definition replacing the embedded one of the same name.
func loadBaseBlocks(customBase []byte) (map[string]*yamltemplate.Template, error) {
	embeddedBase, err := templatesFS.ReadFile("templates/base.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded base.tmpl: %w", err)
	}
	disc, err := yamltemplate.New("base.tmpl").Funcs(templateFuncs()).Parse(string(embeddedBase))
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded base.tmpl: %w", err)
	}
	if len(customBase) > 0 {
		if disc, err = disc.Parse(string(customBase)); err != nil {
			return nil, fmt.Errorf("failed to parse custom base.tmpl: %w", err)
		}
	}
	blocks := make(map[string]*yamltemplate.Template)
	for _, sub := range disc.Templates() {
		if sub.Name() == "base.tmpl" {
			continue
		}
		if blocks[sub.Name()], err = compileSingleBaseBlock(sub.Name(), embeddedBase, customBase); err != nil {
			return nil, err
		}
	}
	return blocks, nil
}

func compileSingleBaseBlock(blockName string, embeddedBase, customBase []byte) (*yamltemplate.Template, error) {
	// Compile a fresh yamltemplate root per block instead of cloning a shared one: safetext binds the
	// StructuralData callback FuncMap to the UUID minted by New(), and Clone()/Lookup() mint a new UUID
	// without rebinding it, so a cloned block fails every execution with ErrInvalidYAMLTemplate. The root
	// body is then replaced so top-level YAML '#' comments in base.tmpl are never emitted.
	bt, err := yamltemplate.New("base_entry_" + blockName).Funcs(templateFuncs()).Parse(string(embeddedBase))
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded base.tmpl for block %q: %w", blockName, err)
	}
	if len(customBase) > 0 {
		if bt, err = bt.Parse(string(customBase)); err != nil {
			return nil, fmt.Errorf("failed to parse custom base.tmpl for block %q: %w", blockName, err)
		}
	}
	if bt, err = bt.Parse("{{- template \"" + blockName + "\" . -}}"); err != nil {
		return nil, fmt.Errorf("failed to compile base.tmpl block %q: %w", blockName, err)
	}
	return bt, nil
}

// ExtractDefaultTemplates writes all embedded Kubernetes manifest templates to targetDir.
func ExtractDefaultTemplates(targetDir string, overwrite bool) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	entries, err := fs.ReadDir(templatesFS, "templates")
	if err != nil {
		return fmt.Errorf("failed to read embedded templates: %w", err)
	}

	if !overwrite {
		if err := ensureNoExistingTemplateFiles(targetDir, entries); err != nil {
			return err
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		destPath := filepath.Join(targetDir, entry.Name())
		data, err := templatesFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read embedded file %q: %w", entry.Name(), err)
		}
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write file %q: %w", destPath, err)
		}
		logging.Info("Extracted template: %s", destPath)
	}
	return nil
}

func ensureNoExistingTemplateFiles(targetDir string, entries []fs.DirEntry) error {
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		destPath := filepath.Join(targetDir, entry.Name())
		if _, err := os.Stat(destPath); err == nil {
			return fmt.Errorf("file %q already exists; extract to a new --output-dir to diff and merge your customizations, or pass --overwrite to replace all files in %q", destPath, targetDir)
		}
	}
	return nil
}
