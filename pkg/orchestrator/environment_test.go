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
	"sync"
	"testing"
)

func TestEnvironment_IsKnownAirGappedAndDefaultOverride(t *testing.T) {
	t.Parallel()

	for _, truthy := range []string{"1", "true", "TRUE", " yes "} {
		val := truthy
		env := &Environment{Getenv: func(k string) string {
			if k == AirGappedEnvVar {
				return val
			}
			return ""
		}}
		if !env.IsKnownAirGapped() {
			t.Errorf("IsKnownAirGapped(%q) = false, want true", val)
		}
	}

	overrideVal := ""
	envDef := &Environment{Getenv: func(k string) string {
		if k == AirGappedEnvVar {
			return overrideVal
		}
		return ""
	}}
	if envDef.IsKnownAirGapped() {
		t.Fatalf("expected fresh Environment not to report IsKnownAirGapped")
	}
	envDef.SetDefaultAirGapped()
	if !envDef.IsKnownAirGapped() {
		t.Fatalf("expected SetDefaultAirGapped() to turn on IsKnownAirGapped()")
	}
	for _, falsy := range []string{"0", "false", "FALSE", "no"} {
		overrideVal = falsy
		if envDef.IsKnownAirGapped() {
			t.Errorf("expected explicit GCLUSTER_AIR_GAPPED=%q to override SetDefaultAirGapped()", falsy)
		}
	}
	overrideVal = ""
	envDef.SetDefaultAirGappedState(false)
	if envDef.IsKnownAirGapped() {
		t.Fatalf("expected SetDefaultAirGappedState(false) to clear default air-gap state")
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			envDef.SetDefaultAirGappedState(idx%2 == 0)
			_ = envDef.IsKnownAirGapped()
		}(i)
	}
	wg.Wait()
}
