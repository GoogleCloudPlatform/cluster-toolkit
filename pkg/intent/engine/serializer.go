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

package engine

import (
	"hpc-toolkit/pkg/intent/ast"

	"gopkg.in/yaml.v3"
)

// ExportedBlueprint represents the strict schema expected by the legacy HPC Toolkit deployment engine.
type ExportedBlueprint struct {
	BlueprintName    string                    `yaml:"blueprint_name"`
	Vars             map[string]any            `yaml:"vars,omitempty"`
	DeploymentGroups []ExportedDeploymentGroup `yaml:"deployment_groups"`
}

// ExportedDeploymentGroup represents a single execution group.
type ExportedDeploymentGroup struct {
	Group   string           `yaml:"group"`
	Modules []ExportedModule `yaml:"modules"`
}

// ExportedModule represents a deployable module, stripped of internal intent metadata like 'condition' and 'export'.
type ExportedModule struct {
	ID       string         `yaml:"id"`
	Source   string         `yaml:"source"`
	Kind     string         `yaml:"kind,omitempty"`
	Use      []string       `yaml:"use,omitempty"`
	Outputs  []any          `yaml:"outputs,omitempty"`
	Settings map[string]any `yaml:"settings,omitempty"`
}

// Serialize converts the AST into a valid, deployable YAML string, stripping out internal intent metadata.
func Serialize(name string, vars map[string]any, groups []ast.DeploymentGroup) (string, error) {
	exported := ExportedBlueprint{
		BlueprintName: name,
		Vars:          vars,
	}

	for _, group := range groups {
		eg := ExportedDeploymentGroup{
			Group: group.Group,
		}

		for _, mod := range group.Modules {
			eg.Modules = append(eg.Modules, ExportedModule{
				ID:       mod.ID,
				Source:   mod.Source,
				Kind:     mod.Kind,
				Use:      mod.Use,
				Outputs:  mod.Outputs,
				Settings: mod.Settings,
			})
		}

		if len(eg.Modules) > 0 {
			exported.DeploymentGroups = append(exported.DeploymentGroups, eg)
		}
	}

	data, err := yaml.Marshal(exported)
	if err != nil {
		return "", err
	}

	// Add yaml document separator at the top for convention
	return "---\n" + string(data), nil
}
