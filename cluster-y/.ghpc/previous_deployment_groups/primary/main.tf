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

module "network1" {
  source          = "./modules/embedded/modules/network/vpc"
  deployment_name = var.deployment_name
  project_id      = var.project_id
  region          = var.region
  secondary_ranges = {
    gke-subnet-y = [{
      ip_cidr_range = "10.4.0.0/14"
      range_name    = "pods"
      }, {
      ip_cidr_range = "10.0.32.0/20"
      range_name    = "services"
    }]
  }
  subnetwork_name = "gke-subnet-y"
}

module "gke_cluster" {
  source                  = "./modules/embedded/modules/scheduler/gke-cluster"
  deployment_name         = var.deployment_name
  enable_private_endpoint = false
  labels                  = var.labels
  network_id              = module.network1.network_id
  project_id              = var.project_id
  region                  = var.region
  subnetwork_self_link    = module.network1.subnetwork_self_link
}

module "workload_manager_install" {
  source     = "./modules/embedded/modules/management/kubectl-apply"
  cluster_id = module.gke_cluster.cluster_id
  jobset = {
    install = true
  }
  project_id = var.project_id
}
