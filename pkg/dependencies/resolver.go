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

package dependencies

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DownloadDecision int

const (
	DownloadDecisionAsk DownloadDecision = iota
	DownloadDecisionYes
	DownloadDecisionNo
)

func getBinaryCacheDir(binaryName, version string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user cache dir: %w", err)
	}
	return filepath.Join(cacheDir, "cluster-toolkit", fmt.Sprintf("%s-%s", binaryName, version)), nil
}

// PatchPath unconditionally appends the cache directories for Terraform and Packer
// to the PATH environment variable.
func PatchPath() error {
	tfCacheDir, err := getBinaryCacheDir("terraform", TerraformVersion)
	if err != nil {
		return err
	}

	packerCacheDir, err := getBinaryCacheDir("packer", PackerVersion)
	if err != nil {
		return err
	}

	currentPath := os.Getenv("PATH")
	newPath := currentPath + string(os.PathListSeparator) + tfCacheDir + string(os.PathListSeparator) + packerCacheDir
	os.Setenv("PATH", newPath)

	return nil
}

// EnsureDependencies checks if terraform and packer are accessible in the PATH.
// If not, it handles downloading them according to the decision.
func EnsureDependencies(decision DownloadDecision) error {
	if err := ensureBinary("terraform", TerraformVersion, decision); err != nil {
		return err
	}
	if err := ensureBinary("packer", PackerVersion, decision); err != nil {
		return err
	}
	return nil
}

func ensureBinary(binaryName, version string, decision DownloadDecision) error {
	if _, err := exec.LookPath(binaryName); err == nil {
		return nil
	}

	if err := confirmDownload(binaryName, version, decision); err != nil {
		return err
	}

	binaryCacheDir, err := getBinaryCacheDir(binaryName, version)
	if err != nil {
		return err
	}

	return downloadAndExtract(binaryName, version, binaryCacheDir)
}

func confirmDownload(binaryName, version string, decision DownloadDecision) error {
	if decision == DownloadDecisionNo {
		return fmt.Errorf("%s is missing. Download is explicitly disabled. Enable download by specifying --download-dependencies flag.", binaryName)
	}

	if decision == DownloadDecisionAsk {
		fmt.Printf("%s v%s is missing. Do you want to download it? [y/N]: ", binaryName, version)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			return fmt.Errorf("user declined to download %s", binaryName)
		}
	}

	return nil
}
