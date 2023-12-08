/**
 * Copyright 2022 Google LLC
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

// Package modulereader extracts necessary information from modules
package modulereader

import (
	"fmt"
	"hpc-toolkit/pkg/sourcereader"
	"log"
	"os"
	"path"

	"github.com/hashicorp/go-getter"
	"gopkg.in/yaml.v3"
)

// VarInfo stores information about a module input variables
type VarInfo struct {
	Name        string
	Type        string
	Description string
	Default     interface{}
	Required    bool
}

// OutputInfo stores information about module output values
type OutputInfo struct {
	Name        string
	Description string `yaml:",omitempty"`
	Sensitive   bool   `yaml:",omitempty"`
	// DependsOn   []string `yaml:"depends_on,omitempty"`
}

// UnmarshalYAML supports parsing YAML OutputInfo fields as a simple list of
// strings or as a list of maps directly into OutputInfo struct
func (mo *OutputInfo) UnmarshalYAML(value *yaml.Node) error {
	var name string
	const yamlErrorMsg string = "block beginning at line %d: %s"

	err := value.Decode(&name)
	if err == nil {
		mo.Name = name
		return nil
	}

	var fields map[string]interface{}
	err = value.Decode(&fields)
	if err != nil {
		return fmt.Errorf(yamlErrorMsg, value.Line, "outputs must each be a string or a map{name: string, description: string, sensitive: bool}; "+err.Error())
	}

	err = enforceMapKeys(fields, map[string]bool{
		"name": true, "description": false, "sensitive": false},
	)
	if err != nil {
		return fmt.Errorf(yamlErrorMsg, value.Line, err)
	}

	type rawOutputInfo OutputInfo
	if err := value.Decode((*rawOutputInfo)(mo)); err != nil {
		return fmt.Errorf("line %d: %s", value.Line, err)
	}
	return nil
}

// enforceMapKeys ensures the presence of required keys and absence of unallowed
// keys with a useful error message; input is a map of all allowed keys to a
// boolean that is true when key is required and false when optional
func enforceMapKeys(input map[string]interface{}, allowedKeys map[string]bool) error {
	for key := range input {
		if _, ok := allowedKeys[key]; !ok {
			return fmt.Errorf("provided invalid key: %#v", key)
		}
		allowedKeys[key] = false
	}
	for key, req := range allowedKeys {
		if req {
			return fmt.Errorf("missing required key: %#v", key)
		}
	}
	return nil
}

// ModuleInfo stores information about a module
type ModuleInfo struct {
	Inputs   []VarInfo
	Outputs  []OutputInfo
	Metadata Metadata
}

// GetOutputsAsMap returns the outputs list as a map for quicker access
func (i ModuleInfo) GetOutputsAsMap() map[string]OutputInfo {
	outputsMap := make(map[string]OutputInfo)
	for _, output := range i.Outputs {
		outputsMap[output.Name] = output
	}
	return outputsMap
}

type sourceAndKind struct {
	source string
	kind   string
}

var modInfoCache = map[sourceAndKind]ModuleInfo{}

// GetModuleInfo gathers information about a module at a given source using the
// tfconfig package. It will add details about required APIs to be
// enabled for that module.
// There is a cache to avoid re-reading the module info for the same source and kind.
func GetModuleInfo(source string, kind string) (ModuleInfo, error) {
	key := sourceAndKind{source, kind}
	if mi, ok := modInfoCache[key]; ok {
		return mi, nil
	}

	var modPath string
	switch {
	case sourcereader.IsEmbeddedPath(source) || sourcereader.IsLocalPath(source):
		modPath = source
	default:
		tmpDir, err := os.MkdirTemp("", "module-*")
		if err != nil {
			return ModuleInfo{}, err
		}
		pkgAddr, subDir := getter.SourceDirSubdir(source)
		pkgPath := path.Join(tmpDir, "module")
		modPath = path.Join(pkgPath, subDir)
		sourceReader := sourcereader.Factory(pkgAddr)
		if err = sourceReader.GetModule(pkgAddr, pkgPath); err != nil {
			if subDir != "" && kind == "packer" {
				err = fmt.Errorf("module source %s included \"//\" package syntax; "+
					"the \"//\" should typically be placed at the root of the repository:\n%w", source, err)
			}
			return ModuleInfo{}, err
		}
	}

	reader := Factory(kind)
	mi, err := reader.GetInfo(modPath)
	if err != nil {
		return ModuleInfo{}, err
	}
	mi.Metadata = GetMetadataSafe(modPath)
	modInfoCache[key] = mi
	return mi, nil
}

// SetModuleInfo sets the ModuleInfo for a given source and kind
// NOTE: This is only used for testing
func SetModuleInfo(source string, kind string, info ModuleInfo) {
	modInfoCache[sourceAndKind{source, kind}] = info
}

// ModReader is a module reader interface
type ModReader interface {
	GetInfo(path string) (ModuleInfo, error)
}

var kinds = map[string]ModReader{
	"terraform": NewTFReader(),
	"packer":    NewPackerReader(),
}

// Factory returns a ModReader of type 'kind'
func Factory(kind string) ModReader {
	r, ok := kinds[kind]
	if !ok {
		log.Fatalf("Invalid request to create a reader of kind %s", kind)
	}
	return r
}
