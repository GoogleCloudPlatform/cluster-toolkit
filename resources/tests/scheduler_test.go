// Copyright 2021 Google LLC
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

package restests

import "testing"

func TestSlurmOnGCPPartition_InitAndValidateSucceeds(t *testing.T) {
	terraformDirRelativeToRoot := "third-party/compute/SchedMD-slurm-on-gcp-partition"
	testInitAndValidate(t, rootDir, terraformDirRelativeToRoot)
}

func TestSlurmOnGCPController_InitAndValidateSucceeds(t *testing.T) {
	terraformDirRelativeToRoot := "third-party/scheduler/SchedMD-slurm-on-gcp-controller"
	testInitAndValidate(t, rootDir, terraformDirRelativeToRoot)
}

func TestSlurmOnGCPLoginNode_InitAndValidateSucceeds(t *testing.T) {
	terraformDirRelativeToRoot := "third-party/scheduler/SchedMD-slurm-on-gcp-login-node"
	testInitAndValidate(t, rootDir, terraformDirRelativeToRoot)
}
