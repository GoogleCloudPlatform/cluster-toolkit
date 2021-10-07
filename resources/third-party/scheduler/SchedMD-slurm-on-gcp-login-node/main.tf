/**
 * Copyright 2021 Google LLC
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

module "slurm_cluster_login_node" {
  source            = "github.com/SchedMD/slurm-gcp//tf/modules/login/?ref=v4.0.4"
  boot_disk_size    = var.boot_disk_size
  boot_disk_type    = var.boot_disk_type
  image             = var.login_image
  instance_template = var.login_instance_template
  cluster_name = (
    var.cluster_name != null
    ? var.cluster_name
    : "slurm-${var.deployment_name}"
  )
  controller_name           = var.controller_name
  controller_secondary_disk = var.controller_secondary_disk
  disable_login_public_ips  = var.disable_login_public_ips
  labels                    = var.labels
  login_network_storage     = var.login_network_storage
  machine_type              = var.login_machine_type
  munge_key                 = var.munge_key
  network_storage           = var.network_storage
  node_count                = var.login_node_count
  region                    = var.region
  scopes                    = var.login_scopes
  service_account           = var.login_service_account
  shared_vpc_host_project   = var.shared_vpc_host_project
  subnet_depend             = var.subnet_depend
  subnetwork_name           = var.subnetwork_name
  zone                      = var.zone
}
