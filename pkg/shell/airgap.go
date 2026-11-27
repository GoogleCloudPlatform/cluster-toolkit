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

package shell

import (
	"os"
	"strings"
	"sync/atomic"
)

// AirGappedEnvVar names the environment variable ("GCLUSTER_AIR_GAPPED") that skips live API server version probes during orchestrator detection, automatic GitHub manifest downloads, and CLI telemetry collection and uploads in air-gapped environments.
const AirGappedEnvVar = "GCLUSTER_AIR_GAPPED"

var defaultAirGap atomic.Bool

// IsKnownAirGapped reports whether air-gapped mode is active from GCLUSTER_AIR_GAPPED or the process default without performing network or subprocess I/O.
func IsKnownAirGapped() bool {
	return IsAirGappedEnv(os.Getenv)
}

// ParseBoolEnv parses a human-friendly boolean environment variable value.
// Recognizes 1/true/yes/on as (true, true) and 0/false/no/off as (false, true),
// returning (false, false) for empty or unrecognized values.
func ParseBoolEnv(raw string) (val, ok bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

// IsAirGappedEnv reports whether air-gapped mode is active. An explicit boolean value in GCLUSTER_AIR_GAPPED
// from getenv (or os.Getenv if nil) overrides the process default; unset, blank, or unrecognized values fall back to the default.
func IsAirGappedEnv(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	if v, ok := ParseBoolEnv(getenv(AirGappedEnvVar)); ok {
		return v
	}
	return defaultAirGap.Load()
}

// SetDefaultAirGapped sets or clears the process-default air-gapped state used when GCLUSTER_AIR_GAPPED is unset, blank, or unrecognized.
func SetDefaultAirGapped(v bool) {
	defaultAirGap.Store(v)
}
