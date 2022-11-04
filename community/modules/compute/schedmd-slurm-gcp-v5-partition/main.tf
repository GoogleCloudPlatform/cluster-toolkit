/**
 * Copyright 2022 Google LLC
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

locals {

  # Default to value in partition_conf if both set "Default"
  partition_conf = merge(var.is_default == true ? { "Default" : "YES" } : {}, var.partition_conf)

  default_node_group = {
    # Group Definition
    group_name             = "ghpc"
    node_count_dynamic_max = var.node_count_dynamic_max
    node_count_static      = var.node_count_static
    node_conf              = var.node_conf

    # Template By Definition
    additional_disks         = var.additional_disks
    bandwidth_tier           = var.bandwidth_tier
    can_ip_forward           = var.can_ip_forward
    disable_smt              = var.disable_smt
    disk_auto_delete         = var.disk_auto_delete
    disk_labels              = var.labels
    disk_size_gb             = var.disk_size_gb
    disk_type                = var.disk_type
    enable_confidential_vm   = var.enable_confidential_vm
    enable_oslogin           = var.enable_oslogin
    enable_shielded_vm       = var.enable_shielded_vm
    gpu                      = var.gpu
    labels                   = var.labels
    machine_type             = var.machine_type
    metadata                 = var.metadata
    min_cpu_platform         = var.min_cpu_platform
    on_host_maintenance      = var.on_host_maintenance
    preemptible              = var.preemptible
    shielded_instance_config = var.shielded_instance_config
    source_image_family      = var.source_image_family == null ? "" : var.source_image_family
    source_image_project     = var.source_image_project == null ? "" : var.source_image_project
    source_image             = var.source_image == null ? "" : var.source_image
    tags                     = var.tags
    service_account = var.service_account != null ? var.service_account : {
      email  = data.google_compute_default_service_account.default.email
      scopes = ["https://www.googleapis.com/auth/cloud-platform"]
    }

    # Spot VM settings
    enable_spot_vm       = var.enable_spot_vm
    spot_instance_config = var.spot_instance_config

    # Template By Source
    instance_template = null
  }

  partition_nodes = concat([local.default_node_group], var.node_groups)

  # Since deployment name may be used to create a cluster name, we remove any invalid character from the beginning
  # Also, slurm imposed a lot of restrictions to this name, so we format it to an acceptable string
  tmp_cluster_name   = substr(replace(lower(var.deployment_name), "/^[^a-z]*|[^a-z0-9]/", ""), 0, 8)
  slurm_cluster_name = var.slurm_cluster_name != null ? var.slurm_cluster_name : local.tmp_cluster_name

  uses_zone_policies = length(var.zone_policy_allow) + length(var.zone_policy_deny) > 0
  excluded_zones     = var.zone == null ? [] : [for z in data.google_compute_zones.available.names : z if z != var.zone]
  zone_policy_deny   = local.uses_zone_policies ? var.zone_policy_deny : local.excluded_zones
  zone_policy_allow  = local.uses_zone_policies || var.zone == null ? var.zone_policy_allow : [var.zone]
}

data "google_compute_zones" "available" {
  project = var.project_id
  region  = var.region
}

data "google_compute_default_service_account" "default" {
  project = var.project_id
}

module "slurm_partition" {
  source = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_partition?ref=v5.1.0"

  slurm_cluster_name      = local.slurm_cluster_name
  partition_nodes         = local.partition_nodes
  enable_job_exclusive    = var.exclusive
  enable_placement_groups = var.enable_placement
  network_storage         = var.network_storage
  partition_name          = var.partition_name
  project_id              = var.project_id
  region                  = var.region
  zone_policy_allow       = local.zone_policy_allow
  zone_policy_deny        = local.zone_policy_deny
  subnetwork              = var.subnetwork_self_link == null ? "" : var.subnetwork_self_link
  partition_conf          = local.partition_conf
}
