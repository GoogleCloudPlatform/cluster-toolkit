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
	"encoding/json"
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
	newPath := tfCacheDir + string(os.PathListSeparator) + packerCacheDir + string(os.PathListSeparator) + currentPath
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
	path, err := exec.LookPath(binaryName)
	if err == nil {
		if binaryName == "terraform" {
			installedVersion, err := getInstalledTfVersion(path)
			if err != nil {
				fmt.Printf("Warning: Could not determine installed terraform version: %v. Proceeding to download recommended version %s.\n", err, version)
			} else {
				cmp, err := compareVersions(installedVersion, version)
				if err != nil {
					fmt.Printf("Warning: Could not parse installed terraform version %q: %v. Proceeding to download recommended version %s.\n", installedVersion, err, version)
				} else if cmp == 0 {
					return nil // exact match
				} else if cmp > 0 {
					// installed version is newer
					fmt.Printf("WARNING: Terraform version %s is currently installed. We recommend using version %s for compatibility with all features.\n", installedVersion, version)
					return nil // proceed with newer version
				} else {
					// installed version is older (incompatible)
					fmt.Printf("Installed terraform version %s is older than required version %s.\n", installedVersion, version)
				}
			}
		} else {
			return nil // for packer, just check existence for now
		}
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

func getInstalledTfVersion(path string) (string, error) {
	cmd := exec.Command(path, "version", "--json")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var version struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal(out, &version); err != nil {
		return "", err
	}

	return version.TerraformVersion, nil
}

// compareVersions returns:
//
//	-1 if v1 < v2
//	 0 if v1 == v2
//	 1 if v1 > v2
func compareVersions(v1, v2 string) (int, error) {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// Strip pre-release info
	v1 = strings.Split(v1, "-")[0]
	v2 = strings.Split(v2, "-")[0]

	var major1, minor1, patch1 int
	var major2, minor2, patch2 int

	_, err := fmt.Sscanf(v1, "%d.%d.%d", &major1, &minor1, &patch1)
	if err != nil {
		return 0, fmt.Errorf("invalid version format %q: %w", v1, err)
	}
	_, err = fmt.Sscanf(v2, "%d.%d.%d", &major2, &minor2, &patch2)
	if err != nil {
		return 0, fmt.Errorf("invalid version format %q: %w", v2, err)
	}

	if major1 != major2 {
		if major1 < major2 {
			return -1, nil
		}
		return 1, nil
	}
	if minor1 != minor2 {
		if minor1 < minor2 {
			return -1, nil
		}
		return 1, nil
	}
	if patch1 != patch2 {
		if patch1 < patch2 {
			return -1, nil
		}
		return 1, nil
	}
	return 0, nil
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
