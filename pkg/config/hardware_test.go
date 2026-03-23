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

package config

import (
	"testing"
)

func TestCalculateTPUNodes(t *testing.T) {
	tests := []struct {
		name          string
		machineType   string
		topology      string
		expectedNodes int
		expectErr     bool
	}{
		{
			name:          "v4 standard 4 chips per VM",
			machineType:   "ct4p-hightpu-4t",
			topology:      "4x4x4",
			expectedNodes: 16, // 64 / 4
			expectErr:     false,
		},
		{
			name:          "v5p standard 4 chips per VM",
			machineType:   "ct5p-hightpu-4t",
			topology:      "4x4x4",
			expectedNodes: 16, // 64 / 4
			expectErr:     false,
		},
		{
			name:          "v5litepod 8 chips per VM",
			machineType:   "ct5lp-hightpu-8t",
			topology:      "8x16",
			expectedNodes: 16, // 128 / 8
			expectErr:     false,
		},
		{
			name:          "v5litepod string literal 8 chips per VM",
			machineType:   "v5litepod-16",
			topology:      "4x4",
			expectedNodes: 2, // 16 / 8
			expectErr:     false,
		},
		{
			name:          "v6e 4 chips per VM",
			machineType:   "ct6e-standard-4t",
			topology:      "2x2",
			expectedNodes: 1, // 4 / 4
			expectErr:     false,
		},
		{
			name:          "v7x 1 chip per VM test",
			machineType:   "tpu7x-standard-1t",
			topology:      "1x1x1",
			expectedNodes: 1, // 1 / 1
			expectErr:     false,
		},
		{
			name:          "v7x 4 chip per VM test",
			machineType:   "tpu7x-standard-4t",
			topology:      "4x4x4",
			expectedNodes: 16, // 64 / 4
			expectErr:     false,
		},
		{
			name:          "not divisible error",
			machineType:   "ct5p-hightpu-4t",
			topology:      "2x1x1", // 2 chips
			expectedNodes: 0,
			expectErr:     true,
		},
		{
			name:          "invalid topology format",
			machineType:   "ct5lp-hightpu-8t",
			topology:      "8x16xfoo",
			expectedNodes: 0,
			expectErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodes, err := calculateTPUNodes(tc.machineType, tc.topology)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if nodes != tc.expectedNodes {
					t.Errorf("expected %d nodes, got %d", tc.expectedNodes, nodes)
				}
			}
		})
	}
}
