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
    gke-subnet-a3-highgpu = [{
      ip_cidr_range = "10.4.0.0/14"
      range_name    = "pods"
      }, {
      ip_cidr_range = "10.0.32.0/20"
      range_name    = "services"
    }]
  }
  subnetwork_name = "gke-subnet-a3-highgpu"
}

module "gpunets" {
  source                  = "./modules/embedded/modules/network/multivpc"
  deployment_name         = var.deployment_name
  global_ip_address_range = "192.169.0.0/16"
  network_count           = 4
  network_name_prefix     = "${var.deployment_name}-gpunet"
  project_id              = var.project_id
  region                  = var.region
  subnetwork_cidr_suffix  = 24
}

module "gke_cluster" {
  source                  = "./modules/embedded/modules/scheduler/gke-cluster"
  additional_networks     = flatten([module.gpunets.additional_networks])
  deployment_name         = var.deployment_name
  enable_private_endpoint = false
  labels                  = var.labels
  master_authorized_networks = [{
    cidr_block   = var.authorized_cidr
    display_name = "kubectl-access-network"
  }]
  network_id           = module.network1.network_id
  project_id           = var.project_id
  region               = var.region
  subnetwork_self_link = module.network1.subnetwork_self_link
}

module "a3_highgpu_pool" {
  source                      = "./modules/embedded/modules/compute/gke-node-pool"
  additional_networks         = flatten([module.gpunets.additional_networks])
  autoscaling_total_min_nodes = 2
  cluster_id                  = module.gke_cluster.cluster_id
  labels                      = var.labels
  machine_type                = "a3-highgpu-8g"
  project_id                  = var.project_id
  zones                       = [var.zone]
}

module "workload_component_install" {
  source     = "./modules/embedded/modules/management/kubectl-apply"
  cluster_id = module.gke_cluster.cluster_id
  jobset = {
    install = true
  }
  kueue = {
    install = true
  }
  project_id = var.project_id
}
