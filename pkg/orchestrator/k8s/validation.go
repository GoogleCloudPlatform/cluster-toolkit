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

package k8s

import (
	"fmt"
	"math"
)

// ValidateNonNegativeInt32 checks that a flag backed by a Kubernetes int32
// field falls within 0..math.MaxInt32.
func ValidateNonNegativeInt32(flagName string, value int) error {
	if value < 0 {
		return fmt.Errorf("%s must be non-negative", flagName)
	}
	if int64(value) > math.MaxInt32 {
		return fmt.Errorf("%s must be at most %d", flagName, math.MaxInt32)
	}
	return nil
}
