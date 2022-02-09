// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resreader

import (
	"fmt"
	"os"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

// getHCLInfo is wrapped by SourceReader interface which supports multiple sources and stores remote resources locally,
// so the given source parameter to getHCLInfo is only a local path.
func getHCLInfo(source string) (ResourceInfo, error) {
	ret := ResourceInfo{}

	fileInfo, err := os.Stat(source)
	if os.IsNotExist(err) {
		return ret, fmt.Errorf("Source to resource does not exist: %s", source)
	}
	if err != nil {
		return ret, fmt.Errorf("Failed to read source of resource: %s", source)
	}
	if !fileInfo.IsDir() {
		return ret, fmt.Errorf("Source of resource must be a directory: %s", source)
	}
	if !tfconfig.IsModuleDir(source) {
		return ret, fmt.Errorf("Source is not a terraform or packer module: %s", source)
	}

	module, _ := tfconfig.LoadModule(source)
	var vars []VarInfo
	var outs []VarInfo
	for _, v := range module.Variables {
		vInfo := VarInfo{
			Name:        v.Name,
			Type:        v.Type,
			Description: v.Description,
			Default:     v.Default,
			Required:    v.Required,
		}
		vars = append(vars, vInfo)
	}
	ret.Inputs = vars
	for _, v := range module.Outputs {
		vInfo := VarInfo{
			Name:        v.Name,
			Description: v.Description,
		}
		outs = append(outs, vInfo)
	}
	ret.Outputs = outs
	return ret, nil
}
