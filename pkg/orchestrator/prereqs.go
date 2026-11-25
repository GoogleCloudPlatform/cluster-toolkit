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

import "slices"

// PrereqKind identifies a local workstation prerequisite check required by an orchestrator.
type PrereqKind string

const (
	// PrereqKubectl requires the kubectl CLI binary in PATH.
	PrereqKubectl PrereqKind = "kubectl"
	// PrereqGKE requires the GKE workstation prerequisite suite (gcloud, auth, ADC, project, kubectl, and gke-gcloud-auth-plugin).
	PrereqGKE PrereqKind = "gke"
)

// BasicPrerequisites returns the workstation prerequisites declared by the provider registered for t.
func BasicPrerequisites(t Type) []PrereqKind {
	registryMu.RLock()
	spec, ok := providerSpecs[t]
	registryMu.RUnlock()
	if !ok {
		return nil
	}
	return slices.Clone(spec.Prerequisites)
}
