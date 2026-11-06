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
	"sync"
	"testing"
)

func TestAirGappedPrecedenceAndConcurrency(t *testing.T) {
	t.Cleanup(func() { SetDefaultAirGappedState(false) })
	SetDefaultAirGappedState(false)

	for _, truthy := range []string{"1", "true", "TRUE", " yes "} {
		val := truthy
		if !IsAirGappedEnv(func(k string) string {
			if k == AirGappedEnvVar {
				return val
			}
			return ""
		}) {
			t.Errorf("IsAirGappedEnv(%q) = false, want true", val)
		}
	}

	t.Setenv(AirGappedEnvVar, "true")
	if !IsAirGappedEnv(nil) {
		t.Fatalf("expected IsAirGappedEnv(nil) to default to os.Getenv and return true")
	}

	t.Setenv(AirGappedEnvVar, "")
	if IsKnownAirGapped() {
		t.Fatalf("expected unset %s with defaultAirGap=false to return false", AirGappedEnvVar)
	}

	SetDefaultAirGapped()
	if !IsKnownAirGapped() {
		t.Fatalf("expected SetDefaultAirGapped() to turn on IsKnownAirGapped()")
	}

	for _, falsy := range []string{"0", "false", "FALSE", "no"} {
		t.Setenv(AirGappedEnvVar, falsy)
		if IsKnownAirGapped() {
			t.Errorf("expected explicit %s=%q to override SetDefaultAirGapped()", AirGappedEnvVar, falsy)
		}
	}

	t.Setenv(AirGappedEnvVar, "")
	SetDefaultAirGappedState(false)
	if IsKnownAirGapped() {
		t.Fatalf("expected SetDefaultAirGappedState(false) to clear default air-gap state")
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			SetDefaultAirGappedState(idx%2 == 0)
			_ = IsKnownAirGapped()
		}(i)
	}
	wg.Wait()
}
