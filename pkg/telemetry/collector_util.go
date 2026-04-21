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
	"fmt"
	"hpc-toolkit/pkg/config"
	"os"
	"os/exec"
	"regexp"

	"strings"

	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	resourcemanagerpb "cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"

	billing "cloud.google.com/go/billing/apiv1"
	"cloud.google.com/go/billing/apiv1/billingpb"

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

func getAllModulesInBp(bp config.Blueprint) []string {
	moduleInfos := config.GetAllModules(&bp)
	modules := make([]string, len(moduleInfos))
	for i, module := range moduleInfos {
		modules[i] = string(module.Source)
	}
	return modules
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

func ifModulesMatchPatterns(modulesList []string, patterns []string) string {
	for _, m := range modulesList {
		for _, p := range patterns {
			if strings.Contains(m, p) {
				return "true"
			}
		}
	}
	return "false"
}

func getKeyFromBlueprint(key string, bp config.Blueprint) string {
	val, err := bp.Eval(config.GlobalRef(key).AsValue())
	if err == nil {
		v, _ := val.Unmark()
		if !v.IsNull() && v.Type() == cty.String {
			return v.AsString()
		}
	}
	return ""
}

// getProjectBillingAccount fetches the billing account associated with a given GCP project in the format "billingAccounts/{billing_account_id}". If billing is disabled for the project, this will return an empty string.
var getProjectBillingAccount = func(ctx context.Context, projectID string) string {
	client, err := billing.NewCloudBillingClient(ctx)
	if err != nil {
		return ""
	}
	defer client.Close()
	req := &billingpb.GetProjectBillingInfoRequest{
		Name: fmt.Sprintf("projects/%s", projectID),
	}
	info, err := client.GetProjectBillingInfo(ctx, req)
	if err != nil {
		return ""
	}
	return info.GetBillingAccountName()
}

// fetchProjectName retrieves the project name (which contains the project number) for a given project ID.
var fetchProjectName = func(ctx context.Context, projectID string) (string, error) {
	client, err := resourcemanager.NewProjectsClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()
	req := &resourcemanagerpb.GetProjectRequest{Name: fmt.Sprintf("projects/%s", projectID)}
	project, err := client.GetProject(ctx, req)
	if err != nil {
		return "", err
	}
	return project.Name, nil
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

// getMacVersion uses sw_vers to get the macOS product version.
func getMacVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout2Sec)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
	if err != nil {
		return "Darwin (unknown version)"
	}
	return strings.TrimSpace(string(out))
}

// getWindowsVersion uses the ver command to get the Windows version.
func getWindowsVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout2Sec)
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
