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
	"os"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/intent"

	"gopkg.in/yaml.v3"
)

// v2 intent compiler telemetry.

// v2 runs are reported through the standard metrics plus a single IS_V2 flag, so every
// existing dashboard chart can be filtered to v2 usage without new metrics.
func IsV2Invocation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	target := args[0]
	path := resolveBlueprintPath(target)
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	if path != target {
		// Deployment folder: path now points at the expanded blueprint.
		return hasV2Marker(data)
	}
	return intent.IsV2Config(data)
}

// hasV2Marker reports whether an expanded blueprint was produced by the v2 compiler.
func hasV2Marker(expandedBlueprint []byte) bool {
	var m struct {
		V2ConfigBase string `yaml:"v2_config_base"`
	}
	if err := yaml.Unmarshal(expandedBlueprint, &m); err != nil {
		return false
	}
	return m.V2ConfigBase != ""
}

// SetBlueprint replaces the collector's blueprint.
//
// Must be called *after* bp.Expand(). Blueprint.Expand has a pointer receiver and mutates
// in place, so a copy taken before it runs still carries unresolved $(vars.*) references
// and would yield empty or wrong values for the standard metrics.
func (c *Collector) SetBlueprint(bp config.Blueprint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blueprint = bp
}

// v2BlueprintName is the value reported in the standard BLUEPRINT metric for v2 runs.
//
// Without it, v2 runs report "Custom": the compiler names the generated blueprint after
// the config base (e.g. "gke"), which is absent from the shipped examples list, so
// getBlueprintName masks it as user-authored. The "Custom" bucket would then silently
// inflate as v2 adoption grows.
const v2BlueprintName = "v2"

// blueprintNameForTelemetryLocked returns the value for the standard BLUEPRINT metric.
// Non-v2 runs are unchanged: they fall through to getBlueprintName.
//
// The caller must already hold c.mu.
func (c *Collector) blueprintNameForTelemetryLocked() string {
	if c.isV2 {
		return v2BlueprintName
	}
	return getBlueprintName(c.blueprint)
}
