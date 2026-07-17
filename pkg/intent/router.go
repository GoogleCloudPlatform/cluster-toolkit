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

// Package intent provides the v2 intent compiler engine for Cluster Toolkit.
package intent

import (
	"os"

	"gopkg.in/yaml.v3"
)

// =========================================================================
// API ROUTER (Legacy V1 vs V2)
// =========================================================================

// IsV2Config acts as the main traffic controller when a user runs `gcluster create`.
// It inspects the AST payload to determine whether to route the compilation to the old
// legacy V1 static engine, or the new V2 intent compiler engine.
//
// Currently, this implements a "Structural Heuristic" (duck-typing). It desperately checks
// the top-level YAML keys for v2-specific fields (`config_base`, `features`, etc.) to
// guess the version.
//
// ARCHITECTURAL DEBT / FUTURE STATE:
// Instead of this fragile heuristic approach, a far superior method is to implement explicit
// File Headers. By requiring users to define a metadata envelope (e.g., `apiVersion: toolkit/v2`
// or `kind: IntentBlueprint`), the router can execute an immediate, deterministic switch statement
// rather than unmarshalling and guessing based on payload shape.
func IsV2Config(content []byte) bool {
	var m map[string]interface{}
	if err := yaml.Unmarshal(content, &m); err != nil {
		return false
	}
	_, hasBase := m["config_base"]
	_, hasArchetypes := m["compute_archetypes"]
	_, hasFeatures := m["features"]
	_, hasOverlays := m["overlays"]
	return hasBase || hasArchetypes || hasFeatures || hasOverlays
}

// IsV2ConfigFile reads the file at path and returns true if it represents a v2 user config.
func IsV2ConfigFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return IsV2Config(data), nil
}
