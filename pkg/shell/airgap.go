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

// AirGappedEnvVar controls Layer-2 air-gap behavior ("true" skips external probes and Clearcut telemetry).
const AirGappedEnvVar = "GCLUSTER_AIR_GAPPED"

var defaultAirGap atomic.Bool

// IsKnownAirGapped reports whether air-gapped mode is active from GCLUSTER_AIR_GAPPED or default state without probing.
func IsKnownAirGapped() bool {
	return IsAirGappedEnv(os.Getenv)
}

// IsAirGappedEnv evaluates air-gapped state using getenv (defaulting to os.Getenv when nil) before falling back to defaultAirGap.
func IsAirGappedEnv(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	switch strings.ToLower(strings.TrimSpace(getenv(AirGappedEnvVar))) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	return defaultAirGap.Load()
}

// SetDefaultAirGapped marks the process environment as air-gapped unless GCLUSTER_AIR_GAPPED=false is explicitly set.
func SetDefaultAirGapped() {
	SetDefaultAirGappedState(true)
}

// SetDefaultAirGappedState sets or clears the process-default air-gapped state.
func SetDefaultAirGappedState(v bool) {
	defaultAirGap.Store(v)
}
