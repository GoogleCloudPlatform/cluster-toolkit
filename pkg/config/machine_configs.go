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

package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"hpc-toolkit/pkg/gcloud"

	"github.com/zclconf/go-cty/cty"
)

func extractStringSetting(m *Module, bp Blueprint, key string) string {
	if m.Settings.Has(key) {
		v, ok := attemptEvalModuleInput(m.Settings.Get(key), bp)
		if ok && v.Type() == cty.String {
			return v.AsString()
		}
	}
	return ""
}

func extractMachineParams(m *Module, bp Blueprint) (string, string, string) {
	machineType := extractStringSetting(m, bp, "machine_type")

	zone := extractStringSetting(m, bp, "zone")
	if zone == "" && bp.Vars.Has("zone") {
		v, ok := attemptEvalModuleInput(bp.Vars.Get("zone"), bp)
		if ok && v.Type() == cty.String {
			zone = v.AsString()
		}
	}

	project := extractStringSetting(m, bp, "project_id")
	if project == "" && bp.Vars.Has("project_id") {
		v, ok := attemptEvalModuleInput(bp.Vars.Get("project_id"), bp)
		if ok && v.Type() == cty.String {
			project = v.AsString()
		}
	}
	return machineType, zone, project
}

func getMachineConfigJSON(m *Module, bp Blueprint) (string, error) {
	machineType, zone, project := extractMachineParams(m, bp)

	if machineType == "" || zone == "" || project == "" {
		return `{"gpus": {}, "tpus": {}}`, nil
	}

	out, err := gcloud.RunGcloudJsonCommand("compute", "machine-types", "describe", machineType, "--zone", zone, "--project", project)
	if err != nil {
		return "", fmt.Errorf("failed to get machine type info for machine_type=%s zone=%s project=%s: %w", machineType, zone, project, err)
	}

	var mt struct {
		GuestCpus    int `json:"guestCpus"`
		Accelerators []struct {
			GuestAcceleratorCount int    `json:"guestAcceleratorCount"`
			GuestAcceleratorType  string `json:"guestAcceleratorType"`
		} `json:"accelerators"`
	}

	if err := json.Unmarshal(out, &mt); err != nil {
		return "", fmt.Errorf("failed to parse gcloud output for machine_type=%s: %w", machineType, err)
	}

	cpuSection := fmt.Sprintf(`"%s": {"count": %d}`, machineType, mt.GuestCpus)

	if strings.HasPrefix(machineType, "ct") || strings.HasPrefix(machineType, "tpu") {
		count := 0
		parts := strings.Split(machineType, "-")
		if len(parts) > 0 {
			suffix := parts[len(parts)-1]
			if strings.HasSuffix(suffix, "t") {
				if _, err := fmt.Sscanf(suffix, "%dt", &count); err != nil {
					count = 0
				}
			}
		}
		if count > 0 {
			return fmt.Sprintf(`{"gpus": {}, "tpus": {"%s": {"count": %d}}, "cpus": {%s}}`, machineType, count, cpuSection), nil
		}
	} else if len(mt.Accelerators) > 0 {
		return fmt.Sprintf(`{"gpus": {"%s": {"count": %d, "type": "%s"}}, "tpus": {}, "cpus": {%s}}`, machineType, mt.Accelerators[0].GuestAcceleratorCount, mt.Accelerators[0].GuestAcceleratorType, cpuSection), nil
	}

	return fmt.Sprintf(`{"gpus": {}, "tpus": {}, "cpus": {%s}}`, cpuSection), nil
}
