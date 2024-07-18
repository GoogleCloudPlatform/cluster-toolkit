// Copyright 2023 "Google LLC"
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

package validators

import (
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"os/exec"
	"strings"
)

func testTfVersionForSlurm(bp config.Blueprint, _ config.Dict) error {
	slurm := false
	bp.WalkModulesSafe(func(_ config.ModulePath, m *config.Module) {
		if strings.HasSuffix(m.Source, "slurm-gcp-v6-controller") {
			slurm = true
		}
	})

	if !slurm {
		return nil
	}

	ver, err := tfVersion()
	if err != nil {
		return nil
	}

	if ver <= "1.4.0" {
		return nil
	}

	return fmt.Errorf("using a newer version of Terraform can lead to controller replacement on reconfigure for Slurm GCP v6\n\n" +
		"Please be advised of this known issue: https://github.com/GoogleCloudPlatform/hpc-toolkit/issues/2774\n" +
		"Until resolved it is advised to use Terraform 1.4.0 with Slurm deployments.\n\n" +
		"To silence this warning, add flag: --skip-validators=test_tf_version_for_slurm")

}

func tfVersion() (string, error) {
	path, err := exec.LookPath("terraform")
	if err != nil {
		return "", err
	}

	out, err := exec.Command(path, "version", "--json").Output()
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
