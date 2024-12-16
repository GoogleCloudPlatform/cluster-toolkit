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
  nodeset_map_ell = { for x in var.nodeset : x.nodeset_name => x... }
  nodeset_map     = { for k, vs in local.nodeset_map_ell : k => vs[0] }

  nodeset_tpu_map_ell = { for x in var.nodeset_tpu : x.nodeset_name => x... }
  nodeset_tpu_map     = { for k, vs in local.nodeset_tpu_map_ell : k => vs[0] }

  nodeset_dyn_map_ell = { for x in var.nodeset_dyn : x.nodeset_name => x... }
  nodeset_dyn_map     = { for k, vs in local.nodeset_dyn_map_ell : k => vs[0] }
}

# NODESET
# TODO: remove dependency on slurm-gcp repo, move to local template module
module "slurm_nodeset_template" {
  source   = "../../internal/slurm-gcp/instance_template"
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
  metadata                 = merge(each.value.metadata, local.universe_domain)
  min_cpu_platform         = each.value.min_cpu_platform
  name_prefix              = each.value.nodeset_name
  on_host_maintenance      = each.value.on_host_maintenance
  preemptible              = each.value.preemptible
  spot                     = each.value.spot
  termination_action       = each.value.termination_action
  service_account          = each.value.service_account
  shielded_instance_config = each.value.shielded_instance_config
  source_image_family      = each.value.source_image_family
  source_image_project     = each.value.source_image_project
  source_image             = each.value.source_image
  subnetwork               = each.value.subnetwork_self_link
  additional_networks      = each.value.additional_networks
  access_config            = each.value.access_config
  tags                     = concat([local.slurm_cluster_name], each.value.tags)
}

module "nodeset_cleanup" {
  source   = "./modules/cleanup_compute"
  for_each = local.nodeset_map

  nodeset                = each.value
  project_id             = var.project_id
  slurm_cluster_name     = local.slurm_cluster_name
  enable_cleanup_compute = var.enable_cleanup_compute
  universe_domain        = var.universe_domain
  endpoint_versions      = var.endpoint_versions
  gcloud_path_override   = var.gcloud_path_override
}

locals {
  nodesets = [for name, ns in local.nodeset_map : {
    nodeset_name                     = ns.nodeset_name
    node_conf                        = ns.node_conf
    dws_flex                         = ns.dws_flex
    instance_template                = module.slurm_nodeset_template[ns.nodeset_name].self_link
    node_count_dynamic_max           = ns.node_count_dynamic_max
    node_count_static                = ns.node_count_static
    subnetwork                       = ns.subnetwork_self_link
    reservation_name                 = ns.reservation_name
    future_reservation               = ns.future_reservation
    maintenance_interval             = ns.maintenance_interval
    instance_properties_json         = ns.instance_properties_json
    enable_placement                 = ns.enable_placement
    placement_max_distance           = ns.placement_max_distance
    network_storage                  = ns.network_storage
    zone_target_shape                = ns.zone_target_shape
    zone_policy_allow                = ns.zone_policy_allow
    zone_policy_deny                 = ns.zone_policy_deny
    enable_maintenance_reservation   = ns.enable_maintenance_reservation
    enable_opportunistic_maintenance = ns.enable_opportunistic_maintenance
  }]
}

# NODESET TPU
module "slurm_nodeset_tpu" {
  source   = "../../internal/slurm-gcp/nodeset_tpu"
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

module "nodeset_cleanup_tpu" {
  source   = "./modules/cleanup_tpu"
  for_each = local.nodeset_tpu_map

  nodeset = {
    nodeset_name = each.value.nodeset_name
    zone         = each.value.zone
  }

  project_id             = var.project_id
  slurm_cluster_name     = local.slurm_cluster_name
  enable_cleanup_compute = var.enable_cleanup_compute
  universe_domain        = var.universe_domain
  endpoint_versions      = var.endpoint_versions
  gcloud_path_override   = var.gcloud_path_override

  depends_on = [
    # Depend on controller network, as a best effort to avoid
    # subnetwork resourceInUseByAnotherResource error
    var.subnetwork_self_link
  ]
}
