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

##########
# LOCALS #
##########

locals {
  boot_disk = {
    source_image = local.source_image
    disk_size_gb = var.disk_size_gb
    disk_type    = var.disk_type
    disk_labels  = var.disk_labels
    auto_delete  = var.disk_auto_delete
    boot         = "true"
  }

  service_account = {
    email  = try(var.service_account.email, null)
    scopes = try(var.service_account.scopes, ["https://www.googleapis.com/auth/cloud-platform"])
  }


  name_prefix = "${var.slurm_cluster_name}-${var.slurm_instance_role}-${var.name_prefix}"

  total_egress_bandwidth_tier = var.bandwidth_tier == "tier_1_enabled" ? "TIER_1" : "DEFAULT"

  nic_type_map = {
    platform_default = null
    virtio_enabled   = "VIRTIO_NET"
    gvnic_enabled    = "GVNIC"
    tier_1_enabled   = "GVNIC"
  }
  nic_type = lookup(local.nic_type_map, var.bandwidth_tier, null)

  automatic_updates_metadata = var.allow_automatic_updates ? {} : { google_disable_automatic_updates = "TRUE" }
}

########
# DATA #
########

data "local_file" "startup" {
  filename = "${path.module}/files/startup_sh_unlinted"
}

############
# TEMPLATE #
############

module "instance_template" {
  source = "../internal_instance_template"

  project_id = var.project_id

  # Network
  can_ip_forward              = var.can_ip_forward
  network_ip                  = var.network_ip
  network                     = var.network
  nic_type                    = local.nic_type
  region                      = var.region
  subnetwork_project          = var.subnetwork_project
  subnetwork                  = var.subnetwork
  tags                        = var.tags
  total_egress_bandwidth_tier = local.total_egress_bandwidth_tier
  additional_networks         = var.additional_networks
  access_config               = var.access_config

  # Instance
  machine_type             = var.machine_type
  min_cpu_platform         = var.min_cpu_platform
  name_prefix              = local.name_prefix
  gpu                      = var.gpu
  service_account          = local.service_account
  shielded_instance_config = var.shielded_instance_config
  threads_per_core         = var.disable_smt ? 1 : null
  enable_confidential_vm   = var.enable_confidential_vm
  enable_shielded_vm       = var.enable_shielded_vm
  preemptible              = var.preemptible
  spot                     = var.spot
  on_host_maintenance      = var.on_host_maintenance
  labels = merge(
    var.labels,
    {
      slurm_cluster_name  = var.slurm_cluster_name
      slurm_instance_role = var.slurm_instance_role
    },
  )
  instance_termination_action = var.termination_action

  # Metadata
  startup_script = data.local_file.startup.content
  metadata = merge(
    var.metadata,
    {
      enable-oslogin      = upper(var.enable_oslogin)
      slurm_bucket_path   = var.slurm_bucket_path
      slurm_cluster_name  = var.slurm_cluster_name
      slurm_instance_role = var.slurm_instance_role
    },
    local.automatic_updates_metadata,
  )

  # Disk  
  disks = concat([local.boot_disk], var.additional_disks)
  disks_labels = {
    slurm_cluster_name  = var.slurm_cluster_name
    slurm_instance_role = var.slurm_instance_role
  }
}
