/**
  * Copyright 2023 Google LLC
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

module "network" {
  source          = "./modules/embedded/modules/network/vpc"
  deployment_name = var.deployment_name
  labels          = var.labels
  project_id      = var.project_id
  region          = var.region
}

module "private_service_access" {
  source     = "./modules/embedded/community/modules/network/private-service-access"
  labels     = var.labels
  network_id = module.network.network_id
  project_id = var.project_id
}

module "homefs" {
  source            = "./modules/embedded/modules/file-system/filestore"
  connect_mode      = module.private_service_access.connect_mode
  deployment_name   = var.deployment_name
  labels            = var.labels
  local_mount       = "/home"
  network_id        = module.network.network_id
  project_id        = var.project_id
  region            = var.region
  reserved_ip_range = module.private_service_access.reserved_ip_range
  zone              = var.zone
}

module "debug_nodeset" {
  source                  = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-nodeset"
  allow_automatic_updates = false
  labels                  = var.labels
  machine_type            = "n2-standard-2"
  name                    = "debug_nodeset"
  node_count_dynamic_max  = 4
  project_id              = var.project_id
  region                  = var.region
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "debug_partition" {
  source         = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-partition"
  exclusive      = false
  is_default     = true
  nodeset        = flatten([module.debug_nodeset.nodeset])
  partition_name = "debug"
}

module "compute_nodeset" {
  source                  = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-nodeset"
  allow_automatic_updates = false
  bandwidth_tier          = "gvnic_enabled"
  labels                  = var.labels
  name                    = "compute_nodeset"
  node_count_dynamic_max  = 20
  project_id              = var.project_id
  region                  = var.region
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "compute_partition" {
  source         = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-partition"
  nodeset        = flatten([module.compute_nodeset.nodeset])
  partition_name = "compute"
}

module "h3_nodeset" {
  source                  = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-nodeset"
  allow_automatic_updates = false
  bandwidth_tier          = "gvnic_enabled"
  disk_type               = "pd-balanced"
  labels                  = var.labels
  machine_type            = "h3-standard-88"
  name                    = "h3_nodeset"
  node_count_dynamic_max  = 20
  project_id              = var.project_id
  region                  = var.region
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "h3_partition" {
  source         = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-partition"
  nodeset        = flatten([module.h3_nodeset.nodeset])
  partition_name = "h3"
}

module "slurm_login" {
  source                  = "./modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-login"
  enable_login_public_ips = true
  labels                  = var.labels
  machine_type            = "n2-standard-4"
  name_prefix             = "slurm_login"
  project_id              = var.project_id
  region                  = var.region
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "slurm_controller" {
  source                       = "./modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-controller"
  deployment_name              = var.deployment_name
  enable_controller_public_ips = true
  labels                       = var.labels
  login_nodes                  = flatten([module.slurm_login.login_nodes])
  network_storage              = flatten([module.homefs.network_storage])
  nodeset                      = flatten([module.h3_partition.nodeset, flatten([module.compute_partition.nodeset, flatten([module.debug_partition.nodeset])])])
  nodeset_dyn                  = flatten([module.h3_partition.nodeset_dyn, flatten([module.compute_partition.nodeset_dyn, flatten([module.debug_partition.nodeset_dyn])])])
  nodeset_tpu                  = flatten([module.h3_partition.nodeset_tpu, flatten([module.compute_partition.nodeset_tpu, flatten([module.debug_partition.nodeset_tpu])])])
  partitions                   = flatten([module.h3_partition.partitions, flatten([module.compute_partition.partitions, flatten([module.debug_partition.partitions])])])
  project_id                   = var.project_id
  region                       = var.region
  subnetwork_self_link         = module.network.subnetwork_self_link
  zone                         = var.zone
}
