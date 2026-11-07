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

package orchestrator_test

import (
	"slices"
	"testing"

	"hpc-toolkit/pkg/orchestrator"
	_ "hpc-toolkit/pkg/orchestrator/gke"
)

func TestBasicPrerequisites(t *testing.T) {
	t.Parallel()

	k8sKinds := orchestrator.BasicPrerequisites(orchestrator.TypeK8s)
	if !slices.Equal(k8sKinds, []orchestrator.PrereqKind{orchestrator.PrereqKubectl}) {
		t.Fatalf("BasicPrerequisites(TypeK8s) = %v, want [kubectl]", k8sKinds)
	}

	gkeKinds := orchestrator.BasicPrerequisites(orchestrator.TypeGKE)
	wantGKE := []orchestrator.PrereqKind{
		orchestrator.PrereqGCloudSDK,
		orchestrator.PrereqGCloudAuth,
		orchestrator.PrereqADC,
		orchestrator.PrereqKubectl,
		orchestrator.PrereqGKEAuthPlugin,
		orchestrator.PrereqGCPProject,
	}
	if !slices.Equal(gkeKinds, wantGKE) {
		t.Fatalf("BasicPrerequisites(TypeGKE) = %v, want %v", gkeKinds, wantGKE)
	}

	if got := orchestrator.BasicPrerequisites("unregistered"); got != nil {
		t.Fatalf("BasicPrerequisites(unregistered) = %v, want nil", got)
	}
}
