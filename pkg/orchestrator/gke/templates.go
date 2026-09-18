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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"

	"hpc-toolkit/pkg/logging"

	"github.com/google/safetext/yamltemplate"
)

func (g *GKEOrchestrator) getTemplatePath() (string, error) {
	if g != nil && g.gkeCustomTemplatesPath != "" {
		stat, err := os.Stat(g.gkeCustomTemplatesPath)
		if err != nil {
			return "", fmt.Errorf("custom template directory specified in flag %q could not be accessed: %w", g.gkeCustomTemplatesPath, err)
		}
		if !stat.IsDir() {
			return "", fmt.Errorf("custom template directory specified in flag %q is not a directory", g.gkeCustomTemplatesPath)
		}
		return g.gkeCustomTemplatesPath, nil
	}
	return "", nil
}

func (g *GKEOrchestrator) parseGKETemplate(name string) (*yamltemplate.Template, error) {
	customPath, err := g.getTemplatePath()
	if err != nil {
		return nil, err
	}
	if customPath != "" {
		localFile := filepath.Join(customPath, name)
		if _, err := os.Stat(localFile); err == nil {
			logging.Info("Loading GKE template %q from custom path %q", name, customPath)
			dirFS := os.DirFS(customPath)
			return yamltemplate.New(name).ParseFS(dirFS, name)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to access custom template %q: %w", localFile, err)
		}
		logging.Info("Template %q not found in custom path %q. Falling back to embedded default.", name, customPath)
	}
	return yamltemplate.New(name).ParseFS(templatesFS, "templates/"+name)
}

func (g *GKEOrchestrator) parseGKETextTemplate(name string) (*template.Template, error) {
	customPath, err := g.getTemplatePath()
	if err != nil {
		return nil, err
	}
	if customPath != "" {
		localFile := filepath.Join(customPath, name)
		if _, err := os.Stat(localFile); err == nil {
			logging.Info("Loading GKE text template %q from custom path %q", name, customPath)
			dirFS := os.DirFS(customPath)
			return template.ParseFS(dirFS, name)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to access custom text template %q: %w", localFile, err)
		}
		logging.Info("Text template %q not found in custom path %q. Falling back to embedded default.", name, customPath)
	}
	return template.ParseFS(templatesFS, "templates/"+name)
}

// ExtractDefaultTemplates writes all embedded templates to targetDir.
func ExtractDefaultTemplates(targetDir string, overwrite bool) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	entries, err := fs.ReadDir(templatesFS, "templates")
	if err != nil {
		return fmt.Errorf("failed to read embedded templates: %w", err)
	}

	// Pre-check to avoid partial extraction if overwrite is disabled
	if !overwrite {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			destPath := filepath.Join(targetDir, entry.Name())
			if _, err := os.Stat(destPath); err == nil {
				return fmt.Errorf("file %q already exists. Use --overwrite to replace it", destPath)
			}
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
