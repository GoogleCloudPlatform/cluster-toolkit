// Copyright 2026 "Google LLC"
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

package telemetry

import (
	"bufio"
	"context"
	"hpc-toolkit/pkg/config"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/zclconf/go-cty/cty"
)

func getBlueprint(args []string) config.Blueprint {
	if len(args) == 0 {
		return config.Blueprint{}
	}
	bp, _, _ := config.NewBlueprint(args[0])
	return bp
}

func getEventMetadataKVPairs(sourceMetadata map[string]string) []map[string]string {
	eventMetadata := make([]map[string]string, 0)
	for k, v := range sourceMetadata {
		eventMetadata = append(eventMetadata, map[string]string{
			"key":   k,
			"value": v,
		})
	}
	return eventMetadata
}

func getModulesWithPattern(pattern string, bp config.Blueprint) []config.Module {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	modules := make([]config.Module, 0)
	for _, m := range config.GetAllModules(&bp) {
		if re.MatchString(m.Source) {
			modules = append(modules, m)
		}
	}
	return modules
}

func getKeyFromBlueprint(key string, bp config.Blueprint) string {
	val, err := bp.Eval(config.GlobalRef(key).AsValue())
	if err != nil {
		return ""
	}
	v, _ := val.Unmark()
	if !v.IsNull() && v.Type() == cty.String {
		return v.AsString()
	}
	return ""
}

// getLinuxVersion parses /etc/os-release to find the pretty name or version ID.
func getLinuxVersion() string {
	// Standard way to identify Linux distribution version
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "Linux (unknown version)"
	}
	defer f.Close()

	var prettyName, versionID string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			prettyName = parseOsReleaseField(line)
		} else if strings.HasPrefix(line, "VERSION_ID=") {
			versionID = parseOsReleaseField(line)
		}
	}

	if prettyName != "" {
		return prettyName
	}
	if versionID != "" {
		return versionID
	}
	return "Linux (unknown version)"
}

const (
	versionTimeOut = 2 * time.Second
)

// getMacVersion uses sw_vers to get the macOS product version.
func getMacVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeOut)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
	if err != nil {
		return "Darwin (unknown version)"
	}
	return strings.TrimSpace(string(out))
}

// getWindowsVersion uses the ver command to get the Windows version.
func getWindowsVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeOut)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cmd", "/c", "ver")
	out, err := cmd.Output()
	if err != nil {
		return "Windows (unknown version)"
	}
	return strings.TrimSpace(string(out))
}

// parseOsReleaseField helper to clean up quotes from /etc/os-release values
func parseOsReleaseField(line string) string {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.Trim(parts[1], "'\"")
}
