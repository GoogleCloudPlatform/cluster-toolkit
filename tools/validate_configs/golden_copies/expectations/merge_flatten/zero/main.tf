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
  project_id      = var.project_id
  region          = var.region
}

module "first-fs" {
  source          = "./modules/embedded/modules/file-system/filestore"
  deployment_name = var.deployment_name
  labels          = var.labels
  local_mount     = "/first"
  network_id      = module.network.network_id
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
}

module "second-fs" {
  source          = "./modules/embedded/modules/file-system/filestore"
  deployment_name = var.deployment_name
  labels          = var.labels
  local_mount     = "/first"
  network_id      = module.network.network_id
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
}

module "first-vm" {
  source          = "./modules/embedded/modules/compute/vm-instance"
  deployment_name = var.deployment_name
  labels = merge(var.labels, {
    green = "sleeves"
  })
  network_storage = flatten([module.first-fs.network_storage])
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
}

module "second-vm" {
  source          = "./modules/embedded/modules/compute/vm-instance"
  deployment_name = var.deployment_name
  labels          = var.labels
  network_storage = flatten([module.second-fs.network_storage, flatten([module.first-fs.network_storage])])
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
}
