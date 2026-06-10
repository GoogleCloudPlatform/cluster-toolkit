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

package shell

import (
	"bytes"
	"hpc-toolkit/pkg/config"
	"io"
	"os"
	"os/exec"
)

// ConfigurePacker errors if packer is not in the user PATH
func ConfigurePacker() error {
	_, err := exec.LookPath("packer")
	if err != nil {
		return config.HintError{
			Hint: "must have a copy of packer installed in PATH (obtain at https://packer.io)",
			Err:  err}
	}
	return nil
}

// ExecPackerCmd runs packer with arguments in the given working directory
// optionally prints to stdout/stderr
func ExecPackerCmd(workingDir string, printToScreen bool, args ...string) error {
	cmd := exec.Command("packer", args...)
	cmd.Dir = workingDir

	// Create buffers to capture output
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer

	// Assign writers directly. This lets os/exec manage the streams internally
	// and avoids manual WaitGroup management which can lead to deadlocks.
	if printToScreen {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
	}

	// Run() starts the command and waits for it to finish.
	if err := cmd.Run(); err != nil {
		if !printToScreen {
			// If we weren't printing in real-time, dump the captured
			// output now to help with debugging the failure.
			io.Copy(os.Stdout, &outBuf)
			io.Copy(os.Stderr, &errBuf)
		}
		return err
	}
	return nil
}
