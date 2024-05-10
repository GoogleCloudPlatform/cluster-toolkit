# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "schedmd-slurm-gcp-v6-nodeset-mig", ghpc_role = "compute" })
}

locals {
  name         = substr(replace(var.name, "/[^a-z0-9]/", ""), 0, 14)
  node_feature = coalesce(var.node_feature, local.name)

  service_account = {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["https://www.googleapis.com/auth/cloud-platform"]
  }
}

data "google_compute_default_service_account" "default" {
  project = var.project_id
}

module "instance_template" {
  source = "github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_instance_template?ref=6.5.7&depth=1"

  metadata = {
    slurmd_feature = local.node_feature
  }

  project_id = var.project_id
  region     = var.region
  labels     = local.labels

  slurm_bucket_path   = var.slurm_bucket_path
  slurm_cluster_name  = var.slurm_cluster_name
  slurm_instance_role = "compute"
  subnetwork          = var.subnetwork_self_link

  machine_type    = var.machine_type
  tags            = [var.slurm_cluster_name]
  service_account = local.service_account
}


resource "google_compute_instance_group_manager" "mig" {
  name = "${var.slurm_cluster_name}-${local.name}"

  base_instance_name = "${var.slurm_cluster_name}-${local.name}"
  zone               = var.zone

  version {
    instance_template = module.instance_template.self_link
  }

  target_size = var.target_size
}
