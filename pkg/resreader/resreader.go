/**
 * Copyright 2021 Google LLC
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

// Package resreader extracts necessary information from modules
package resreader

import (
	"log"
)

// VarInfo stores information about a module's input or output variables
type VarInfo struct {
	Name        string
	Type        string
	Description string
	Default     interface{}
	Required    bool
}

// ModuleInfo stores information about a module
type ModuleInfo struct {
	Inputs  []VarInfo
	Outputs []VarInfo
}

// GetOutputsAsMap returns the outputs list as a map for quicker access
func (i ModuleInfo) GetOutputsAsMap() map[string]VarInfo {
	outputsMap := make(map[string]VarInfo)
	for _, output := range i.Outputs {
		outputsMap[output.Name] = output
	}
	return outputsMap
}

// ModReader is a module reader interface
type ModReader interface {
	GetInfo(path string) (ModuleInfo, error)
	SetInfo(path string, modInfo ModuleInfo)
}

var kinds = map[string]ModReader{
	"terraform": TFReader{allModInfo: make(map[string]ModuleInfo)},
	"packer":    PackerReader{allModInfo: make(map[string]ModuleInfo)},
}

// IsValidKind returns true if the kind input is valid
func IsValidKind(input string) bool {
	for k := range kinds {
		if k == input {
			return true
		}
	}
	return false
}

// Factory returns a ModReader of type 'kind'
func Factory(kind string) ModReader {
	for k, v := range kinds {
		if kind == k {
			return v
		}
	}
	log.Fatalf("Invalid request to create a reader of kind %s", kind)
	return nil
}
