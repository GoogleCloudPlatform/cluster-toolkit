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
	"hpc-toolkit/pkg/orchestrator/gke"
)

func TestBasicPrerequisites(t *testing.T) {
	gke.RegisterProviders()
	t.Parallel()

	k8sKinds := orchestrator.BasicPrerequisites(orchestrator.TypeK8s)
	if !slices.Equal(k8sKinds, []orchestrator.PrereqKind{orchestrator.PrereqKubectl}) {
		t.Errorf("BasicPrerequisites(%q) = %v, want [kubectl]", orchestrator.TypeK8s, k8sKinds)
	}

	gkeKinds := orchestrator.BasicPrerequisites(orchestrator.TypeGKE)
	wantGKE := []orchestrator.PrereqKind{orchestrator.PrereqGKE}
	if !slices.Equal(gkeKinds, wantGKE) {
		t.Errorf("BasicPrerequisites(%q) = %v, want %v", orchestrator.TypeGKE, gkeKinds, wantGKE)
	}

	if got := orchestrator.BasicPrerequisites("unregistered"); got != nil {
		t.Errorf("BasicPrerequisites(\"unregistered\") = %v, want nil", got)
	}
}
