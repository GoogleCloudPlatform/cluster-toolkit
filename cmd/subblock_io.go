// Copyright 2026 Google LLC
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

package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/subblockmap"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Overridden in tests.
var (
	subblockStdin      io.Reader = os.Stdin
	subblockIsTerminal           = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	subblockNow                  = time.Now
)

// setSubblockIO lets gke-node-pool modules with pin_to_subblocks: true ask the user
// and save the sub-block map. gcluster expand stays read-only.
func setSubblockIO(cmd *cobra.Command, bp *config.Blueprint, blueprintPath string) {
	if cmd.Name() == "expand" {
		return
	}
	file := blueprintPath
	if expandFlags.deploymentFile != "" {
		file = expandFlags.deploymentFile
	}
	bp.SetSubblockIO(&config.SubblockIO{
		File:    file,
		Confirm: confirmSubblocks,
		Save: func(varName string, m map[string]string) (string, error) {
			return file, saveSubblockMap(file, varName, m)
		},
	})
}

// confirmSubblocks asks a yes/no question. `gcluster deploy --auto-approve`
// answers yes; without a terminal it returns an error with instructions.
func confirmSubblocks(question string) (bool, error) {
	if flagAutoApprove {
		logging.Info("%s yes (--auto-approve)", question)
		return true, nil
	}
	if !subblockIsTerminal() {
		return false, fmt.Errorf("cannot ask %q because gcluster isn't running in a terminal. "+
			"Run it interactively, use `gcluster deploy --auto-approve`, or set the map yourself (YAML above)", question)
	}
	fmt.Printf("%s [y/N]: ", question)
	line, err := bufio.NewReader(subblockStdin).ReadString('\n')
	if err != nil && line == "" {
		return false, fmt.Errorf("reading answer: %w", err)
	}
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes", nil
}

// saveSubblockMap writes `varName: m` under vars: of file, keeping the rest of the
// file. It checks the result, keeps a timestamped backup and prints the diff.
func saveSubblockMap(file, varName string, m map[string]string) error {
	before, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	after, err := subblockmap.UpdateVar(string(before), varName, m)
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	var doc struct {
		Vars map[string]interface{} `yaml:"vars"`
	}
	if err := yaml.Unmarshal([]byte(after), &doc); err != nil {
		return fmt.Errorf("%s: internal check failed, file not changed: %w", file, err)
	}
	got, _ := doc.Vars[varName].(map[string]interface{})
	if len(got) != len(m) {
		return fmt.Errorf("%s: internal check failed, file not changed", file)
	}
	for k, v := range m {
		if got[k] != v {
			return fmt.Errorf("%s: internal check failed for key %q, file not changed", file, k)
		}
	}

	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	backup := fmt.Sprintf("%s.bak-%s", file, subblockNow().Format("20060102-150405"))
	if err := os.WriteFile(backup, before, info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.WriteFile(file, []byte(after), info.Mode().Perm()); err != nil {
		return err
	}
	logging.Info("Saved %s in %s (backup: %s):\n%s", varName, file, backup, subblockmap.Diff(string(before), after, backup, file))
	return nil
}
