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

// VarInfo stores information about a resource's input or output variables
type VarInfo struct {
	Name        string
	Type        string
	Description string
	Default     interface{}
	Required    bool
}

// ResourceInfo stores information about a resource
type ResourceInfo struct {
	Inputs  []VarInfo
	Outputs []VarInfo
}

// GetOutputsAsMap returns the outputs list as a map for quicker access
func (ri ResourceInfo) GetOutputsAsMap() map[string]VarInfo {
	outputsMap := make(map[string]VarInfo)
	for _, output := range ri.Outputs {
		outputsMap[output.Name] = output
	}
	return outputsMap
}

// ResReader is a resource reader interface
type ResReader interface {
	GetInfo(path string) (ResourceInfo, error)
	SetInfo(path string, resInfo ResourceInfo)
}

var kinds = map[string]ResReader{
	"terraform": TFReader{allResInfo: make(map[string]ResourceInfo)},
	"packer":    PackerReader{allResInfo: make(map[string]ResourceInfo)},
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

// Factory returns a ResReader of type 'kind'
func Factory(kind string) ResReader {
	for k, v := range kinds {
		if kind == k {
			return v
		}
	}
	log.Fatalf("Invalid request to create a reader of kind %s", kind)
	return nil
}
