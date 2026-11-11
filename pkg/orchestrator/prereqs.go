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
	// PrereqGCloudSDK requires the Google Cloud SDK (gcloud) binary in PATH.
	PrereqGCloudSDK PrereqKind = "gcloud_sdk"
	// PrereqGCloudAuth requires an active gcloud user/service-account login.
	PrereqGCloudAuth PrereqKind = "gcloud_auth"
	// PrereqADC requires valid Google Cloud Application Default Credentials.
	PrereqADC PrereqKind = "adc"
	// PrereqGKEAuthPlugin requires gke-gcloud-auth-plugin in PATH.
	PrereqGKEAuthPlugin PrereqKind = "gke_gcloud_auth_plugin"
	// PrereqGCPProject verifies access to the configured Google Cloud project.
	PrereqGCPProject PrereqKind = "gcp_project"
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
