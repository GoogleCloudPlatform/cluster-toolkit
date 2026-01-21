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
	"sync"
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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// capture stdout/stderr; print to screen in real-time or upon error
	var wg sync.WaitGroup
	var outBuf io.ReadWriter
	var errBuf io.ReadWriter
	if printToScreen {
		outBuf = os.Stdout
		errBuf = os.Stderr
	} else {
		outBuf = bytes.NewBuffer([]byte{})
		errBuf = bytes.NewBuffer([]byte{})
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(outBuf, stdout)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(errBuf, stderr)
	}()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		if !printToScreen {
			io.Copy(os.Stdout, outBuf)
			io.Copy(os.Stderr, errBuf)
		}
		return err
	}
	return nil
}
