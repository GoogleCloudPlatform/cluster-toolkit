/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"hpc-toolkit/pkg/modulereader"
	"testing"
	"time"
)

func TestValidateDeprecation(t *testing.T) {
	tests := []struct {
		name        string
		info        modulereader.ModuleInfo
		expectError bool
	}{
		{
			name: "no deprecation",
			info: modulereader.ModuleInfo{
				Metadata: modulereader.Metadata{
					Ghpc: modulereader.MetadataGhpc{},
				},
			},
			expectError: false,
		},
		{
			name: "future deprecation date",
			info: modulereader.ModuleInfo{
				Metadata: modulereader.Metadata{
					Ghpc: modulereader.MetadataGhpc{
						DeprecationDate: time.Now().Add(24 * time.Hour).Format("2006-01-02"),
					},
				},
			},
			expectError: false,
		},
		{
			name: "past deprecation date",
			info: modulereader.ModuleInfo{
				Metadata: modulereader.Metadata{
					Ghpc: modulereader.MetadataGhpc{
						DeprecationDate: "2020-01-01",
					},
				},
			},
			expectError: false,
		},
		{
			name: "malformed deprecation date",
			info: modulereader.ModuleInfo{
				Metadata: modulereader.Metadata{
					Ghpc: modulereader.MetadataGhpc{
						DeprecationDate: "yesterday",
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeprecation("test-module", tt.info)
			if (err != nil) != tt.expectError {
				t.Errorf("validateDeprecation() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}
