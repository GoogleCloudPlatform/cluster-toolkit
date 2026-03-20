/**
  * Copyright 2026 Google LLC
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

module "cluster" {
  source                  = "./modules/embedded/modules/scheduler/gke-cluster"
  deployment_name         = var.deployment_name
  enable_slice_controller = true
  labels                  = var.labels
  network_id              = module.network.network_id
  project_id              = var.project_id
  region                  = var.region
  subnetwork_self_link    = module.network.subnetwork_self_link
}
