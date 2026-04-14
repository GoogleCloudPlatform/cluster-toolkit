# Copyright 2026 "Google LLC"
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

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 4.0.0"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = ">= 4.0.0"
    }
  }
}

provider "google" {
  project = "dominikrabij-gke-dev"
  region  = "us-central1"
  zone    = "us-central1-c"
}

provider "google-beta" {
  project = "dominikrabij-gke-dev"
  region  = "us-central1"
  zone    = "us-central1-c"
}

data "google_client_config" "default" {}

data "google_container_cluster" "my_cluster" {
  name     = "test-ss"
  location = "us-central1"
  project  = "dominikrabij-gke-dev"
}

provider "kubernetes" {
  host                   = "https://${data.google_container_cluster.my_cluster.endpoint}"
  token                  = data.google_client_config.default.access_token
  cluster_ca_certificate = base64decode(data.google_container_cluster.my_cluster.master_auth[0].cluster_ca_certificate)
}

provider "helm" {
  kubernetes {
    host                   = "https://${data.google_container_cluster.my_cluster.endpoint}"
    token                  = data.google_client_config.default.access_token
    cluster_ca_certificate = base64decode(data.google_container_cluster.my_cluster.master_auth[0].cluster_ca_certificate)
  }
}

module "vpc" {
  source = "../modules/network/vpc"

  project_id      = "dominikrabij-gke-dev"
  region          = "us-central1"
  deployment_name = "test-ss"
  subnetwork_name = "gke-subnet"

  secondary_ranges = {
    "gke-subnet" = [
      {
        range_name    = "pods"
        ip_cidr_range = "10.4.0.0/14"
      },
      {
        range_name    = "services"
        ip_cidr_range = "10.0.32.0/20"
      }
    ]
  }
}

module "gke_cluster" {
  source = "../modules/scheduler/gke-cluster"

  project_id      = "dominikrabij-gke-dev"
  region          = "us-central1"
  zone            = "us-central1-c"
  deployment_name = "test-ss"

  network_id           = module.vpc.network_id
  subnetwork_self_link = module.vpc.subnetwork_self_link

  pods_ip_range_name     = "pods"
  services_ip_range_name = "services"

  enable_private_endpoint = false
  master_authorized_networks = [
    {
      cidr_block   = "0.0.0.0/0"
      display_name = "all"
    }
  ]
  labels = {}
}

module "kubectl_apply" {
  source = "../modules/management/kubectl-apply"

  project_id = "dominikrabij-gke-dev"
  cluster_id = module.gke_cluster.cluster_id

  kueue = {
    install                 = true
    enable_slice_controller = true
    version                 = "0.15.2"
    controller_cpu          = "1"
    controller_memory       = "1Gi"
  }

  jobset = {
    install           = true
    version           = "0.10.1"
    controller_cpu    = "1"
    controller_memory = "1Gi"
  }
}
