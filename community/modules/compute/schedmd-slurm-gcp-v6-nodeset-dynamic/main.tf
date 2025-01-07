# Copyright 2023 Google LLC
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
  labels = merge(var.labels, { ghpc_module = "schedmd-slurm-gcp-v6-nodeset-dynamic", ghpc_role = "compute" })
}

module "gpu" {
  source = "../../../../modules/internal/gpu-definition"

  machine_type      = var.machine_type
  guest_accelerator = var.guest_accelerator
}

locals {
  guest_accelerator = module.gpu.guest_accelerator

  nodeset_name = substr(replace(var.name, "/[^a-z0-9]/", ""), 0, 14)
  feature      = coalesce(var.feature, local.nodeset_name)

  disable_automatic_updates_metadata = var.allow_automatic_updates ? {} : { google_disable_automatic_updates = "TRUE" }

  metadata = merge(
    local.disable_automatic_updates_metadata,
    { slurmd_feature = local.feature },
    var.metadata
  )

  nodeset = {
    nodeset_name = local.nodeset_name
    nodeset_feature : local.feature
  }

  additional_disks = [
    for ad in var.additional_disks : {
      disk_name    = ad.disk_name
      device_name  = ad.device_name
      disk_type    = ad.disk_type
      disk_size_gb = ad.disk_size_gb
      disk_labels  = merge(ad.disk_labels, local.labels)
      auto_delete  = ad.auto_delete
      boot         = ad.boot
    }
  ]

  public_access_config = var.enable_public_ips ? [{ nat_ip = null, network_tier = null }] : []
  access_config        = length(var.access_config) == 0 ? local.public_access_config : var.access_config

  service_account = {
    email  = var.service_account_email
    scopes = var.service_account_scopes
  }
}

module "slurm_nodeset_template" {
  source = "../../internal/slurm-gcp/instance_template"

  project_id          = var.project_id
  region              = var.region
  name_prefix         = local.nodeset_name
  slurm_cluster_name  = var.slurm_cluster_name
  slurm_instance_role = "compute"
  slurm_bucket_path   = var.slurm_bucket_path
  metadata            = local.metadata

  additional_disks = local.additional_disks
  disk_auto_delete = var.disk_auto_delete
  disk_labels      = merge(local.labels, var.disk_labels)
  disk_size_gb     = var.disk_size_gb
  disk_type        = var.disk_type

  bandwidth_tier = var.bandwidth_tier
  can_ip_forward = var.can_ip_forward

  disable_smt              = !var.enable_smt
  enable_confidential_vm   = var.enable_confidential_vm
  enable_oslogin           = var.enable_oslogin
  enable_shielded_vm       = var.enable_shielded_vm
  shielded_instance_config = var.shielded_instance_config

  labels       = local.labels
  machine_type = var.machine_type

  min_cpu_platform     = var.min_cpu_platform
  on_host_maintenance  = var.on_host_maintenance
  termination_action   = try(var.spot_instance_config.termination_action, null)
  preemptible          = var.preemptible
  spot                 = var.enable_spot_vm
  service_account      = local.service_account
  gpu                  = one(local.guest_accelerator)          # requires gpu_definition.tf
  source_image_family  = local.source_image_family             # requires source_image_logic.tf
  source_image_project = local.source_image_project_normalized # requires source_image_logic.tf
  source_image         = local.source_image                    # requires source_image_logic.tf

  subnetwork          = var.subnetwork_self_link
  additional_networks = var.additional_networks
  access_config       = local.access_config
  tags                = concat([var.slurm_cluster_name], var.tags)
}
