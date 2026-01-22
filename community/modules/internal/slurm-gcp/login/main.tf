# Copyright 2026 Google LLC
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

module "template" {
  source = "../instance_template"

  project_id          = var.project_id
  slurm_cluster_name  = var.slurm_cluster_name
  slurm_instance_role = "login"
  slurm_bucket_path   = var.slurm_bucket_path
  name_prefix         = local.name

  additional_disks           = var.login_nodes.additional_disks
  bandwidth_tier             = var.login_nodes.bandwidth_tier
  can_ip_forward             = var.login_nodes.can_ip_forward
  advanced_machine_features  = var.login_nodes.advanced_machine_features
  disk_auto_delete           = var.login_nodes.disk_auto_delete
  disk_labels                = var.login_nodes.disk_labels
  disk_resource_manager_tags = var.login_nodes.disk_resource_manager_tags
  disk_size_gb               = var.login_nodes.disk_size_gb
  disk_type                  = var.login_nodes.disk_type
  enable_confidential_vm     = var.login_nodes.enable_confidential_vm
  enable_oslogin             = var.login_nodes.enable_oslogin
  enable_shielded_vm         = var.login_nodes.enable_shielded_vm
  gpu                        = var.login_nodes.gpu
  labels                     = var.login_nodes.labels
  machine_type               = var.login_nodes.machine_type
  metadata = merge(var.login_nodes.metadata, {
    "universe_domain"   = var.universe_domain,
    "slurm_login_group" = local.name
  })
  min_cpu_platform         = var.login_nodes.min_cpu_platform
  on_host_maintenance      = var.login_nodes.on_host_maintenance
  preemptible              = var.login_nodes.preemptible
  region                   = var.login_nodes.region
  resource_manager_tags    = var.login_nodes.resource_manager_tags
  service_account          = var.login_nodes.service_account
  shielded_instance_config = var.login_nodes.shielded_instance_config
  source_image_family      = var.login_nodes.source_image_family
  source_image_project     = var.login_nodes.source_image_project
  source_image             = var.login_nodes.source_image
  spot                     = var.login_nodes.spot
  subnetwork               = var.login_nodes.subnetwork
  tags                     = concat([var.slurm_cluster_name], var.login_nodes.tags)
  termination_action       = var.login_nodes.termination_action

  internal_startup_script = var.internal_startup_script
}

module "instance" {
  source = "../instance"

  access_config = var.login_nodes.access_config
  hostname      = "${var.slurm_cluster_name}-${local.name}"

  project_id = var.project_id

  instance_template = module.template.self_link
  num_instances     = var.login_nodes.num_instances

  additional_networks = var.login_nodes.additional_networks
  region              = var.login_nodes.region
  static_ips          = var.login_nodes.static_ips
  subnetwork          = var.login_nodes.subnetwork
  zone                = var.login_nodes.zone

  replace_trigger = var.replace_trigger
}

resource "google_storage_bucket_object" "startup_scripts" {
  for_each = {
    for s in var.startup_scripts : format(
      "slurm-login-%s-script-%s", local.name, replace(basename(s.filename), "/[^a-zA-Z0-9-_]/", "_")
    ) => s.content
  }

  bucket         = var.slurm_bucket_name
  name           = "${var.slurm_bucket_dir}/${each.key}"
  content        = each.value
  source_md5hash = md5(each.value)
}

locals {
  name = var.login_nodes.group_name # short hand

  config = {
    group_name              = local.name
    startup_scripts_timeout = var.startup_scripts_timeout
    network_storage         = var.network_storage
  }
}

resource "google_storage_bucket_object" "config" {
  bucket         = var.slurm_bucket_name
  name           = "${var.slurm_bucket_dir}/login_group_configs/${local.name}.yaml"
  content        = yamlencode(local.config)
  source_md5hash = md5(yamlencode(local.config))

  # To ensure that login group "is not ready" until all startup scripts are written down
  depends_on = [google_storage_bucket_object.startup_scripts]
}
