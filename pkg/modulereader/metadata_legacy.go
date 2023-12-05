// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modulereader

import (
	"strings"
)

func legacyMetadata(source string) Metadata {
	services := []string{}
	if idx := strings.LastIndex(source, "community/modules/"); idx != -1 {
		services = defaultAPIList(source[idx:])
	} else if idx := strings.LastIndex(source, "modules/"); idx != -1 {
		services = defaultAPIList(source[idx:])
	}

	return Metadata{
		Spec: MetadataSpec{
			Requirements: MetadataRequirements{
				Services: services,
			},
		},
	}
}

// DO NOT MODIFY. Specify in metadata.yaml instead.
func defaultAPIList(source string) []string {
	// API lists at
	// https://console.cloud.google.com/apis/dashboard and
	// https://console.cloud.google.com/apis/library
	staticAPIMap := map[string][]string{
		"community/modules/compute/SchedMD-slurm-on-gcp-partition": {
			"compute.googleapis.com",
		},
		"community/modules/compute/htcondor-execute-point": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/compute/pbspro-execution": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/compute/schedmd-slurm-gcp-v5-partition": {
			"compute.googleapis.com",
		},
		"community/modules/database/slurm-cloudsql-federation": {
			"bigqueryconnection.googleapis.com",
			"sqladmin.googleapis.com",
		},
		"community/modules/file-system/DDN-EXAScaler": {
			"compute.googleapis.com",
			"deploymentmanager.googleapis.com",
			"iam.googleapis.com",
			"runtimeconfig.googleapis.com",
		},
		"community/modules/file-system/Intel-DAOS": {
			"compute.googleapis.com",
			"iam.googleapis.com",
			"secretmanager.googleapis.com",
		},
		"community/modules/file-system/nfs-server": {
			"compute.googleapis.com",
		},
		"community/modules/project/new-project": {
			"admin.googleapis.com",
			"cloudresourcemanager.googleapis.com",
			"cloudbilling.googleapis.com",
			"iam.googleapis.com",
		},
		"community/modules/project/service-account": {
			"iam.googleapis.com",
		},
		"community/modules/project/service-enablement": {
			"serviceusage.googleapis.com",
		},
		"community/modules/scheduler/SchedMD-slurm-on-gcp-controller": {
			"compute.googleapis.com",
		},
		"community/modules/scheduler/SchedMD-slurm-on-gcp-login-node": {
			"compute.googleapis.com",
		},
		"community/modules/compute/gke-node-pool": {
			"container.googleapis.com",
		},
		"community/modules/scheduler/gke-cluster": {
			"container.googleapis.com",
		},
		"modules/scheduler/batch-job-template": {
			"batch.googleapis.com",
			"compute.googleapis.com",
		},
		"modules/scheduler/batch-login-node": {
			"batch.googleapis.com",
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scheduler/htcondor-access-point": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scheduler/htcondor-central-manager": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scheduler/htcondor-pool-secrets": {
			"iam.googleapis.com",
			"secretmanager.googleapis.com",
		},
		"community/modules/scheduler/htcondor-setup": {
			"iam.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scheduler/pbspro-client": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scheduler/pbspro-server": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scheduler/schedmd-slurm-gcp-v5-controller": {
			"compute.googleapis.com",
			"iam.googleapis.com",
			"pubsub.googleapis.com",
			"secretmanager.googleapis.com",
		},
		"community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid": {
			"compute.googleapis.com",
			"pubsub.googleapis.com",
		},
		"community/modules/scheduler/schedmd-slurm-gcp-v5-login": {
			"compute.googleapis.com",
		},
		"community/modules/scripts/htcondor-install": {},
		"community/modules/scripts/omnia-install":    {},
		"community/modules/scripts/pbspro-preinstall": {
			"iam.googleapis.com",
			"storage.googleapis.com",
		},
		"community/modules/scripts/pbspro-install": {},
		"community/modules/scripts/pbspro-qmgr":    {},
		"community/modules/scripts/spack-setup": {
			"storage.googleapis.com",
		},
		"community/modules/scripts/wait-for-startup": {
			"compute.googleapis.com",
		},
		"modules/compute/vm-instance": {
			"compute.googleapis.com",
		},
		"modules/file-system/filestore": {
			"file.googleapis.com",
		},
		"modules/file-system/cloud-storage-bucket": {
			"storage.googleapis.com",
		},
		"modules/file-system/pre-existing-network-storage": {},
		"modules/monitoring/dashboard": {
			"stackdriver.googleapis.com",
		},
		"modules/network/pre-existing-vpc": {
			"compute.googleapis.com",
		},
		"modules/network/vpc": {
			"compute.googleapis.com",
		},
		"modules/packer/custom-image": {
			"compute.googleapis.com",
			"storage.googleapis.com",
		},
		"modules/scripts/startup-script": {
			"storage.googleapis.com",
		},
	}

	requiredAPIs, found := staticAPIMap[source]
	if !found {
		return []string{}
	}
	return requiredAPIs
}
