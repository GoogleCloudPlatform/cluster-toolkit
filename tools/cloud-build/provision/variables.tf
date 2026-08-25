/**
  * Copyright 2026 Google LLC
  *
  * Licensed under the Apache License, Version 2.0 (the "License");
  * you may not use this file except in compliance with the License.
  * You may obtain a copy of the License at
  *
  *      http://www.apache.org/licenses/LICENSE-2.0
  *
  * Unless required by applicable law or agreed to in writing, software
  * distributed under the License is distributed on an "AS IS" BASIS,
  * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  * See the License for the specific language governing permissions and
  * limitations under the License.
  */

variable "project_id" {
  description = "GCP project ID"
  type        = string
  default     = "hpc-toolkit-dev"
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "GCP zone"
  type        = string
  default     = "us-central1-c"
}

variable "repo_uri" {
  description = "URI of GitHub repo"
  type        = string
  default     = "https://github.com/GoogleCloudPlatform/cluster-toolkit"
}

variable "daily_tests_project_id" {
  description = "The GCP project for daily tests"
  type        = string
  default     = "hpc-toolkit-dev-2"
}

variable "kueue_migrated_tests" {
  description = "List of tests migrated to Kueue"
  type        = list(string)
  default = [
    "slurm-gcp-v6-rocky8",
    "batch-mpi",
    "htcondor",
    "packer",
    "monitoring",
    "chrome-remote-desktop",
    "chrome-remote-desktop-ubuntu",
    "ansible-vm",
    "e2e",
    "hcls",
    "slurm-gke",
    "slurm-flex",
    "ml-slurm",
    "htc-slurm",
    "hpc-build-slurm-image",
    "hpc-enterprise-slurm",
    "spack-gromacs",
    "gcluster-dockerfile",
    "gke",
    "gke-inactive-reservation",
    "ml-gke",
    "ml-gke-e2e",
    "gke-storage",
    "gke-managed-hyperdisk",
    "slurm-rapid-storage",
    "gke-managed-lustre",
    "pfs-managed-lustre-slurm",
    "pfs-managed-lustre-vm",
    "netapp-volumes",
    "slurm-gcp-v6-reconfig-size",
    "slurm-gcp-v6-simple-job-completion",
    "slurm-gcp-v6-startup-scripts",
    "slurm-gcp-v6-topology",
    "slurm-gcp-v6-debian",
    "slurm-gcp-v6-ubuntu",
    "slurm-gcp-v6-ssd",
    "gke-a2-highgpu-kueue-onspot",
    "gke-a4-onspot",
    "gke-g4-onspot",
    "gke-h4d-onspot",
    "ml-h4d-onspot-slurm",
    "h4d-vm",
    "ml-a3-highgpu-onspot-slurm",
    "ml-a4-highgpu-onspot-slurm",
    "ml-g4-onspot-slurm",
    "gke-a3-highgpu-onspot",
    "gke-a3-megagpu-onspot",
    "gke-tpu-v6e",
    "ml-a3-megagpu-onspot-slurm-ubuntu",
    "gke-a3-ultragpu-onspot",
    "ml-a3-ultragpu-onspot-slurm",
    "ml-a3-ultragpu-onspot-jbvms",
    "gke-a4x",
    "ml-a4x-highgpu-slurm",
    "batch",
    "gke-g4-confidential",
    "gke-tpu-v5e",
    "gke-tpu-7x",
    "gke-tpu-v5p"
  ]
}

variable "daily_tests_service_account" {
  description = "The service account to run daily tests under. If null, the default Cloud Build service account is used. For projects enforcing BYOSA (like hpc-toolkit-dev-2), you must set this via environment variable, e.g. export TF_VAR_daily_tests_service_account=\"projects/...\""
  type        = string
  default     = null
}
