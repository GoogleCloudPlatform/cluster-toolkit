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

package engine_test

import (
	"testing"

	"github.com/zclconf/go-cty/cty"

	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/intent/engine"
	"hpc-toolkit/pkg/modulereader"
)

func TestValidateModuleSettings(t *testing.T) {
	// Mock a module in the cache so we don't need real disk files
	modulereader.SetModuleInfo("test/source", "terraform", modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{
			{Name: "node_count", Type: cty.Number},
			{Name: "machine_type", Type: cty.String},
		},
	})

	tests := []struct {
		name    string
		module  ast.ModuleSpec
		wantErr bool
	}{
		{
			name: "Valid settings",
			module: ast.ModuleSpec{
				ID:     "my_module",
				Source: "test/source",
				Settings: map[string]any{
					"node_count":   10,
					"machine_type": "n2-standard-4",
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid setting triggers error",
			module: ast.ModuleSpec{
				ID:     "my_module",
				Source: "test/source",
				Settings: map[string]any{
					"node_count_typo": 10,
				},
			},
			wantErr: true,
		},
		{
			name: "Missing source is skipped",
			module: ast.ModuleSpec{
				ID: "virtual_module",
				Settings: map[string]any{
					"anything": "goes",
				},
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := engine.ValidateModuleSettings(tc.module, "")
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateModuleSettings() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
