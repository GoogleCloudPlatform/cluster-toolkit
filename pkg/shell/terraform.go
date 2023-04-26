/**
 * Copyright 2023 Google LLC
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

package shell

import (
	"fmt"
	"os/exec"

	"github.com/hashicorp/terraform-exec/tfexec"
)

// TfError captures Terraform errors while improving helpfulness of message
type TfError struct {
	help string
	err  error
}

func (se *TfError) Error() string {
	return fmt.Sprintf("%s (detailed error below)\n%s", se.help, se.err)
}

// ConfigureTerraform returns a Terraform object used to execute commands
func ConfigureTerraform(workingDir string) (*tfexec.Terraform, error) {
	path, err := exec.LookPath("terraform")
	if err != nil {
		return nil, &TfError{
			help: "must have a copy of terraform installed in PATH",
			err:  err,
		}
	}
	return tfexec.NewTerraform(workingDir, path)
}
