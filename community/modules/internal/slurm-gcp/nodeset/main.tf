# Copyright 2025 Google LLC
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


# because 
# module.slurm_controller.module.slurm_nodeset_template["debugnodeset"].module.instance_template.google_compute_instance_template.tpl 
# was moved to 
# module.slurm_controller.module.nodest["debugnodeset"].module.instance_template.google_compute_instance_template.tpl
# , which is not in configuration

module "template" {
  source = "../instance_template"

  project_id          = var.project_id
  slurm_cluster_name  = var.slurm_cluster_name
  slurm_instance_role = "compute"
  slurm_bucket_path   = var.slurm_bucket_path

  additional_disks           = var.nodeset.additional_disks
  bandwidth_tier             = var.nodeset.bandwidth_tier
  can_ip_forward             = var.nodeset.can_ip_forward
  advanced_machine_features  = var.nodeset.advanced_machine_features
  disk_auto_delete           = var.nodeset.disk_auto_delete
  disk_labels                = var.nodeset.disk_labels
  disk_resource_manager_tags = var.nodeset.disk_resource_manager_tags
  disk_size_gb               = var.nodeset.disk_size_gb
  disk_type                  = var.nodeset.disk_type
  enable_confidential_vm     = var.nodeset.enable_confidential_vm
  enable_oslogin             = var.nodeset.enable_oslogin
  enable_shielded_vm         = var.nodeset.enable_shielded_vm
  gpu                        = var.nodeset.gpu
  labels                     = merge(var.nodeset.labels, { slurm_nodeset = var.nodeset.nodeset_name })
  machine_type               = var.nodeset.machine_type
  metadata                   = merge(var.nodeset.metadata, { "universe_domain" = var.universe_domain })
  min_cpu_platform           = var.nodeset.min_cpu_platform
  name_prefix                = var.nodeset.nodeset_name
  on_host_maintenance        = var.nodeset.on_host_maintenance
  preemptible                = var.nodeset.preemptible
  resource_manager_tags      = var.nodeset.resource_manager_tags
  spot                       = var.nodeset.spot
  termination_action         = var.nodeset.termination_action
  service_account            = var.nodeset.service_account
  shielded_instance_config   = var.nodeset.shielded_instance_config
  source_image_family        = var.nodeset.source_image_family
  source_image_project       = var.nodeset.source_image_project
  source_image               = var.nodeset.source_image
  subnetwork                 = var.nodeset.subnetwork_self_link
  additional_networks        = var.nodeset.additional_networks
  access_config              = var.nodeset.access_config
  tags                       = concat([var.slurm_cluster_name], var.nodeset.tags)

  max_run_duration     = (var.nodeset.dws_flex.enabled && !var.nodeset.dws_flex.use_bulk_insert) ? var.nodeset.dws_flex.max_run_duration : null
  provisioning_model   = (var.nodeset.dws_flex.enabled && !var.nodeset.dws_flex.use_bulk_insert) ? "FLEX_START" : null
  reservation_affinity = (var.nodeset.dws_flex.enabled && !var.nodeset.dws_flex.use_bulk_insert) ? { type : "NO_RESERVATION" } : null
}

moved {
  from = module.instance_template
  to   = module.template.module.instance_template
}

locals {
  name = var.nodeset.nodeset_name # just a shorthand

  config = {
    nodeset_name            = local.name
    instance_template       = module.template.self_link
    startup_scripts_timeout = var.startup_scripts_timeout

    node_conf                        = var.nodeset.node_conf
    dws_flex                         = var.nodeset.dws_flex
    network_storage                  = var.nodeset.network_storage
    node_count_dynamic_max           = var.nodeset.node_count_dynamic_max
    node_count_static                = var.nodeset.node_count_static
    subnetwork                       = var.nodeset.subnetwork_self_link
    reservation_name                 = var.nodeset.reservation_name
    future_reservation               = var.nodeset.future_reservation
    maintenance_interval             = var.nodeset.maintenance_interval
    instance_properties_json         = var.nodeset.instance_properties_json
    enable_placement                 = var.nodeset.enable_placement
    placement_max_distance           = var.nodeset.placement_max_distance
    zone_target_shape                = var.nodeset.zone_target_shape
    zone_policy_allow                = var.nodeset.zone_policy_allow
    zone_policy_deny                 = var.nodeset.zone_policy_deny
    enable_maintenance_reservation   = var.nodeset.enable_maintenance_reservation
    enable_opportunistic_maintenance = var.nodeset.enable_opportunistic_maintenance
  }
}

resource "google_storage_bucket_object" "startup_scripts" {
  for_each = {
    for s in var.startup_scripts : format(
      "slurm-nodeset-%s-script-%s", local.name, replace(basename(s.filename), "/[^a-zA-Z0-9-_]/", "_")
    ) => s.content
  }

  bucket  = var.slurm_bucket_name
  name    = "${var.slurm_bucket_dir}/${each.key}"
  content = each.value
}

resource "google_storage_bucket_object" "config" {
  bucket  = var.slurm_bucket_name
  name    = "${var.slurm_bucket_dir}/nodeset_configs/${local.name}.yaml"
  content = yamlencode(local.config)

  depends_on = [google_storage_bucket_object.startup_scripts]
}
