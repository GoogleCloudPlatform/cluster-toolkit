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
  nodeset_map     = { for x in var.nodeset : x.nodeset_name => x }
  nodeset_tpu_map = { for x in var.nodeset_tpu : x.nodeset_name => x }

  partition_map = { for x in var.partitions : x.partition_name => x }
}

# NODESET
module "slurm_nodeset_template" {
  source   = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_instance_template?ref=6.2.0"
  for_each = local.nodeset_map

  project_id          = var.project_id
  slurm_cluster_name  = local.slurm_cluster_name
  slurm_instance_role = "compute"
  slurm_bucket_path   = module.slurm_files.slurm_bucket_path

  additional_disks         = each.value.additional_disks
  bandwidth_tier           = each.value.bandwidth_tier
  can_ip_forward           = each.value.can_ip_forward
  disable_smt              = each.value.disable_smt
  disk_auto_delete         = each.value.disk_auto_delete
  disk_labels              = each.value.disk_labels
  disk_size_gb             = each.value.disk_size_gb
  disk_type                = each.value.disk_type
  enable_confidential_vm   = each.value.enable_confidential_vm
  enable_oslogin           = each.value.enable_oslogin
  enable_shielded_vm       = each.value.enable_shielded_vm
  gpu                      = each.value.gpu
  labels                   = each.value.labels
  machine_type             = each.value.machine_type
  metadata                 = each.value.metadata
  min_cpu_platform         = each.value.min_cpu_platform
  name_prefix              = each.value.nodeset_name
  on_host_maintenance      = each.value.on_host_maintenance
  preemptible              = each.value.preemptible
  service_account          = each.value.service_account
  shielded_instance_config = each.value.shielded_instance_config
  source_image_family      = each.value.source_image_family
  source_image_project     = each.value.source_image_project
  source_image             = each.value.source_image
  subnetwork               = each.value.subnetwork
  tags                     = concat([local.slurm_cluster_name], each.value.tags)
}

module "slurm_nodeset" {
  source   = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_nodeset?ref=6.2.0"
  for_each = local.nodeset_map

  instance_template_self_link = module.slurm_nodeset_template[each.key].self_link

  enable_placement       = each.value.enable_placement
  enable_public_ip       = each.value.enable_public_ip
  network_tier           = each.value.network_tier
  node_count_dynamic_max = each.value.node_count_dynamic_max
  node_count_static      = each.value.node_count_static
  nodeset_name           = each.value.nodeset_name
  node_conf              = each.value.node_conf
  subnetwork_self_link   = each.value.subnetwork
  zones                  = each.value.zones
  zone_target_shape      = each.value.zone_target_shape
}

# NODESET TPU
module "slurm_nodeset_tpu" {
  source   = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_nodeset_tpu?ref=6.2.0"
  for_each = local.nodeset_tpu_map

  project_id             = var.project_id
  node_count_dynamic_max = each.value.node_count_dynamic_max
  node_count_static      = each.value.node_count_static
  nodeset_name           = each.value.nodeset_name
  zone                   = each.value.zone
  node_type              = each.value.node_type
  accelerator_config     = each.value.accelerator_config
  tf_version             = each.value.tf_version
  preemptible            = each.value.preemptible
  preserve_tpu           = each.value.preserve_tpu
  enable_public_ip       = each.value.enable_public_ip
  service_account        = each.value.service_account
  data_disks             = each.value.data_disks
  docker_image           = each.value.docker_image
  subnetwork             = each.value.subnetwork
}

# PARTITION
module "slurm_partition" {
  source   = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_partition?ref=6.2.0"
  for_each = local.partition_map

  partition_nodeset     = [for x in each.value.partition_nodeset : module.slurm_nodeset[x].nodeset_name if try(module.slurm_nodeset[x], null) != null]
  partition_nodeset_tpu = [for x in each.value.partition_nodeset_tpu : module.slurm_nodeset_tpu[x].nodeset_name if try(module.slurm_nodeset_tpu[x], null) != null]

  default              = each.value.default
  enable_job_exclusive = each.value.enable_job_exclusive
  network_storage      = each.value.network_storage
  partition_name       = each.value.partition_name
  partition_conf       = each.value.partition_conf
  resume_timeout       = each.value.resume_timeout
  suspend_time         = each.value.suspend_time
  suspend_timeout      = each.value.suspend_timeout
}
