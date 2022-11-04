// Copyright 2022 Google LLC
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

package sourcereader

import (
	"fmt"
	"hpc-toolkit/pkg/modulereader"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

// ModuleFS contains embedded modules (./modules) for use in building
// blueprints. The main package creates and injects the modules directory as
// hpc-toolkit/modules are not accessible at the package level.
var ModuleFS BaseFS

// BaseFS is an extension of the io.fs interface with the functionality needed
// in CopyDirFromModules. Works with embed.FS and afero.FS
type BaseFS interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
}

// EmbeddedSourceReader reads modules from a local directory
type EmbeddedSourceReader struct{}

// copyDirFromModules copies an FS directory to a local path
func copyDirFromModules(fs BaseFS, source string, dest string) error {
	dirEntries, err := fs.ReadDir(source)
	if err != nil {
		return err
	}
	for _, dirEntry := range dirEntries {
		entryName := dirEntry.Name()
		// path package (not path/filepath) should be used for embedded source
		// as the path separator is a forward slash, even on Windows systems.
		// https://pkg.go.dev/embed#hdr-Directives
		entrySource := path.Join(source, entryName)
		entryDest := filepath.Join(dest, entryName)
		if dirEntry.IsDir() {
			if err := os.Mkdir(entryDest, 0755); err != nil {
				return err
			}
			if err = copyDirFromModules(fs, entrySource, entryDest); err != nil {
				return err
			}
		} else {
			fileBytes, err := fs.ReadFile(entrySource)
			if err != nil {
				return err
			}
			copyFile, err := os.Create(entryDest)
			if err != nil {
				return err
			}
			if _, err = copyFile.Write(fileBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFSToTempDir is a temporary workaround until tfconfig.ReadFromFilesystem
// works against embed.FS.
// Open Issue: https://github.com/hashicorp/terraform-config-inspect/issues/68
func copyFSToTempDir(fs BaseFS, modulePath string) (string, error) {
	tmpDir, err := ioutil.TempDir("", "tfconfig-module-*")
	if err != nil {
		return tmpDir, err
	}
	err = copyDirFromModules(fs, modulePath, tmpDir)
	return tmpDir, err
}

// GetModuleInfo gets modulereader.ModuleInfo for the given kind from the embedded source
func (r EmbeddedSourceReader) GetModuleInfo(modPath string, kind string) (modulereader.ModuleInfo, error) {
	if !IsEmbeddedPath(modPath) {
		return modulereader.ModuleInfo{}, fmt.Errorf("Source is not valid: %s", modPath)
	}

	modDir, err := copyFSToTempDir(ModuleFS, modPath)
	defer os.RemoveAll(modDir)
	if err != nil {
		err = fmt.Errorf("failed to copy embedded module at %s to tmp dir %s: %v",
			modPath, modDir, err)
		return modulereader.ModuleInfo{}, err
	}

	reader := modulereader.Factory(kind)
	mi, err := reader.GetInfo(modDir)
	mi.RequiredApis = defaultAPIList(modPath)
	return mi, err
}

func defaultAPIList(source string) []string {
	// API lists at
	// https://console.cloud.google.com/apis/dashboard and
	// https://console.cloud.google.com/apis/library
	staticAPIMap := map[string][]string{
		"community/modules/compute/SchedMD-slurm-on-gcp-partition": {
			"compute.googleapis.com",
		},
		"community/modules/compute/htcondor-execute-point": {
			"compute.googleapis.com",
		},
		"community/modules/compute/pbspro-execution": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/compute/schedmd-slurm-gcp-v5-partition": {
			"compute.googleapis.com",
		},
		"community/modules/database/slurm-cloudsql-federation": {
			"bigqueryconnection.googleapis.com",
			"sqladmin.googleapis.com",
		},
		"community/modules/file-system/DDN-EXAScaler": {
			"compute.googleapis.com",
			"deploymentmanager.googleapis.com",
			"iam.googleapis.com",
			"runtimeconfig.googleapis.com",
		},
		"community/modules/file-system/Intel-DAOS": {
			"compute.googleapis.com",
			"iam.googleapis.com",
			"secretmanager.googleapis.com",
		},
		"community/modules/file-system/nfs-server": {
			"compute.googleapis.com",
		},
		"community/modules/project/new-project": {
			"admin.googleapis.com",
			"cloudresourcemanager.googleapis.com",
			"cloudbilling.googleapis.com",
			"iam.googleapis.com",
		},
		"community/modules/project/service-account": {
			"iam.googleapis.com",
		},
		"community/modules/project/service-enablement": {
			"serviceusage.googleapis.com",
		},
		"community/modules/scheduler/SchedMD-slurm-on-gcp-controller": {
			"compute.googleapis.com",
		},
		"community/modules/scheduler/SchedMD-slurm-on-gcp-login-node": {
			"compute.googleapis.com",
		},
		"modules/scheduler/batch-job-template": {
			"batch.googleapis.com",
			"compute.googleapis.com",
		},
		"modules/scheduler/batch-login-node": {
			"batch.googleapis.com",
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scheduler/htcondor-configure": {
			"iam.googleapis.com",
			"secretmanager.googleapis.com",
		},
		"community/modules/scheduler/pbspro-client": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scheduler/pbspro-server": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scheduler/schedmd-slurm-gcp-v5-controller": {
			"compute.googleapis.com",
			"iam.googleapis.com",
			"pubsub.googleapis.com",
			"secretmanager.googleapis.com",
		},
		"community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid": {
			"compute.googleapis.com",
			"pubsub.googleapis.com",
		},
		"community/modules/scheduler/schedmd-slurm-gcp-v5-login": {
			"compute.googleapis.com",
		},
		"community/modules/scripts/htcondor-install": {},
		"community/modules/scripts/omnia-install":    {},
		"community/modules/scripts/pbspro-preinstall": {
			"iam.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scripts/pbspro-install": {},
		"community/modules/scripts/pbspro-qmgr":    {},
		"community/modules/scripts/spack-install":  {},
		"community/modules/scripts/wait-for-startup": {
			"compute.googleapis.com",
		},
		"modules/compute/vm-instance": {
			"compute.googleapis.com",
		},
		"modules/file-system/filestore": {
			"file.googleapis.com",
		},
		"modules/file-system/pre-existing-network-storage": {},
		"modules/monitoring/dashboard": {
			"stackdriver.googleapis.com",
		},
		"modules/network/pre-existing-vpc": {
			"compute.googleapis.com",
		},
		"modules/network/vpc": {
			"compute.googleapis.com",
		},
		"modules/packer/custom-image": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"modules/scripts/startup-script": {
			"storage.googleapis.com",
		},
	}

	requiredAPIs, found := staticAPIMap[source]
	if !found {
		return []string{}
	}
	return requiredAPIs
}

// GetModule copies the embedded source to a provided destination (the deployment directory)
func (r EmbeddedSourceReader) GetModule(modPath string, copyPath string) error {
	if !IsEmbeddedPath(modPath) {
		return fmt.Errorf("Source is not valid: %s", modPath)
	}

	modDir, err := copyFSToTempDir(ModuleFS, modPath)
	defer os.RemoveAll(modDir)
	if err != nil {
		err = fmt.Errorf("failed to copy embedded module at %s to tmp dir %s: %v",
			modPath, modDir, err)
		return err
	}

	return copyFromPath(modDir, copyPath)
}
