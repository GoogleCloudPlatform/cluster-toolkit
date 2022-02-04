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
	"io/ioutil"
	"os"

	"hpc-toolkit/pkg/resutils"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

// copyFSToTempDir is a temporary workaround until tfconfig.ReadFromFilesystem
// works against embed.FS.
// Open Issue: https://github.com/hashicorp/terraform-config-inspect/issues/68
func copyFSToTempDir(fs resutils.BaseFS, modulePath string) (string, error) {
	tmpDir, err := ioutil.TempDir("", "tfconfig-module-*")
	if err != nil {
		return tmpDir, err
	}
	err = resutils.CopyDirFromResources(fs, modulePath, tmpDir)
	return tmpDir, err
}

func getHCLInfo(source string) (ResourceInfo, error) {
	ret := ResourceInfo{}

	// Validate source
	var module *tfconfig.Module
	switch {
	case resutils.IsLocalPath(source):
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
			return ret, fmt.Errorf(
				"Source is not a terraform or packer module: %s", source)
		}

		module, _ = tfconfig.LoadModule(source)
	case resutils.IsEmbeddedPath(source):
		resDir, err := copyFSToTempDir(ResourceFS, source)
		defer os.RemoveAll(resDir)
		if err != nil {
			err = fmt.Errorf("failed to copy embedded resource at %s to tmp dir: %v",
				source, err)
			return ret, err
		}
		module, _ = tfconfig.LoadModule(resDir)
	case resutils.IsGitHubPath(source):
		resDir, err := ioutil.TempDir("", "tfconfig-module-*")
		defer os.RemoveAll(resDir)
		if err != nil {
			return ret, err
		}

		if err != resutils.CopyGitHubResources(source, resDir) {
			err = fmt.Errorf("failed to clone GitHub resource at %s to tmp dir: %v",
				source, err)
			return ret, err
		}
		module, _ = tfconfig.LoadModule(resDir)
	default:
		return ret, fmt.Errorf(
			"invalid source (%s), only local and embedded sources are supported",
			source)
	}
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
