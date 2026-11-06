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

package orchestrator

import (
	"os"
	"strings"
	"sync"
)

// AirGappedEnvVar controls Layer-2 air-gap behavior ("true" skips external probes and Clearcut telemetry).
const AirGappedEnvVar = "GCLUSTER_AIR_GAPPED"

// Environment classifies non-probing air-gap state with injectable test seams.
type Environment struct {
	Getenv func(string) string

	mu            sync.Mutex
	defaultAirGap bool
}

var defaultEnv = &Environment{}

// DefaultEnvironment returns the process-default Environment classifier.
func DefaultEnvironment() *Environment {
	return defaultEnv
}

// IsKnownAirGapped reports whether air-gapped mode is active from GCLUSTER_AIR_GAPPED or default state without probing.
func IsKnownAirGapped() bool {
	return defaultEnv.IsKnownAirGapped()
}

// SetDefaultAirGapped marks the default environment as air-gapped unless GCLUSTER_AIR_GAPPED=false is explicitly set.
func SetDefaultAirGapped() {
	SetDefaultAirGappedState(true)
}

// SetDefaultAirGappedState sets or clears the default air-gapped state on the process-default Environment.
func SetDefaultAirGappedState(v bool) {
	defaultEnv.SetDefaultAirGappedState(v)
}

// IsKnownAirGapped reports whether air-gapped mode is known without making any network request.
func (e *Environment) IsKnownAirGapped() bool {
	switch strings.ToLower(strings.TrimSpace(e.getenv()(AirGappedEnvVar))) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.defaultAirGap
}

// SetDefaultAirGapped marks this Environment as air-gapped by default unless GCLUSTER_AIR_GAPPED=false overrides it.
func (e *Environment) SetDefaultAirGapped() {
	e.SetDefaultAirGappedState(true)
}

// SetDefaultAirGappedState sets or clears the default air-gapped state on this Environment.
func (e *Environment) SetDefaultAirGappedState(v bool) {
	e.mu.Lock()
	e.defaultAirGap = v
	e.mu.Unlock()
}

func (e *Environment) getenv() func(string) string {
	if e.Getenv != nil {
		return e.Getenv
	}
	return os.Getenv
}
