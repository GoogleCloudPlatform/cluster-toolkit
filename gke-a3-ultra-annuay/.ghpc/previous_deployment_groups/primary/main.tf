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

terraform {
  backend "gcs" {
    bucket = "tas-terraform"
    prefix = "gke-a3-ultra-annuay/gke-a3-ultra-annuay/primary"
  }
}

module "gke-a3-ultra-annuay-net-0" {
  source          = "github.com/GoogleCloudPlatform/cluster-toolkit.git//modules/network/vpc?ref=e0c690b"
  deployment_name = var.deployment_name
  firewall_rules = [{
    allow = [{
      ports    = ["0-65535"]
      protocol = "tcp"
      }, {
      ports    = ["0-65535"]
      protocol = "udp"
      }, {
      protocol = "icmp"
    }]
    name   = "gke-a3-ultra-annuay-internal-0"
    ranges = ["192.168.0.0/16"]
  }]
  labels       = var.labels
  network_name = "gke-a3-ultra-annuay-net-0"
  project_id   = var.project_id
  region       = var.region
  secondary_ranges = {
    gke-a3-ultra-annuay-sub-0 = [{
      ip_cidr_range = "10.4.0.0/14"
      range_name    = "pods"
      }, {
      ip_cidr_range = "10.0.32.0/20"
      range_name    = "services"
    }]
  }
  subnetworks = [{
    subnet_ip     = "192.168.0.0/18"
    subnet_name   = "gke-a3-ultra-annuay-sub-0"
    subnet_region = var.region
  }]
}

module "gke-a3-ultra-annuay-net-1" {
  source          = "github.com/GoogleCloudPlatform/cluster-toolkit.git//modules/network/vpc?ref=e0c690b"
  deployment_name = var.deployment_name
  firewall_rules = [{
    allow = [{
      ports    = ["0-65535"]
      protocol = "tcp"
      }, {
      ports    = ["0-65535"]
      protocol = "udp"
      }, {
      protocol = "icmp"
    }]
    name   = "gke-a3-ultra-annuay-internal-1"
    ranges = ["192.168.0.0/16"]
  }]
  labels       = var.labels
  mtu          = var.mtu_size
  network_name = "gke-a3-ultra-annuay-net-1"
  project_id   = var.project_id
  region       = var.region
  subnetworks = [{
    subnet_ip     = "192.168.64.0/18"
    subnet_name   = "gke-a3-ultra-annuay-sub-1"
    subnet_region = var.region
  }]
}

module "gke-a3-ultra-annuay-rdma-net" {
  source               = "github.com/GoogleCloudPlatform/cluster-toolkit.git//community/modules/network/rdma-vpc?ref=98c49fe"
  deployment_name      = var.deployment_name
  mtu                  = var.mtu_size
  network_name         = "gke-a3-ultra-annuay-rdma-net"
  network_profile      = "https://www.googleapis.com/compute/beta/projects/${var.project_id}/global/networkProfiles/${var.zone}-vpc-roce"
  network_routing_mode = "REGIONAL"
  project_id           = var.project_id
  region               = var.region
  subnetworks_template = {
    count       = 8
    ip_range    = "192.168.128.0/18"
    name_prefix = "gke-a3-ultra-annuay-rdma-sub"
    region      = var.region
  }
}

module "a3-ultragpu-cluster" {
  source                  = "github.com/GoogleCloudPlatform/cluster-toolkit.git//modules/scheduler/gke-cluster?ref=e0c690b"
  additional_networks     = concat([{ network = module.gke-a3-ultra-annuay-net-1.network_name, subnetwork = module.gke-a3-ultra-annuay-net-1.subnetwork_name, subnetwork_project = var.project_id, nic_type = "GVNIC", queue_count = null, network_ip = null, stack_type = null, access_config = [{ nat_ip = null, public_ptr_domain_name = null, network_tier = null }], ipv6_access_config = [], alias_ip_range = [] }], module.gke-a3-ultra-annuay-rdma-net.subnetwork_interfaces_gke)
  deployment_name         = var.deployment_name
  enable_dcgm_monitoring  = true
  enable_gcsfuse_csi      = true
  enable_private_endpoint = false
  labels                  = var.labels
  maintenance_exclusions = [{
    end_time        = "2025-12-22T00:00:00Z"
    exclusion_scope = "NO_MINOR_OR_NODE_UPGRADES"
    name            = "no-minor-or-node-upgrades-indefinite"
    start_time      = "2024-12-10T00:00:00Z"
  }]
  master_authorized_networks = [{
    cidr_block   = var.authorized_cidr
    display_name = "kubectl-access-network"
  }]
  network_id                    = module.gke-a3-ultra-annuay-net-0.network_id
  project_id                    = var.project_id
  region                        = var.region
  release_channel               = "RAPID"
  subnetwork_self_link          = module.gke-a3-ultra-annuay-net-0.subnetwork_self_link
  system_node_pool_disk_size_gb = var.system_node_pool_disk_size_gb
  system_node_pool_machine_type = "e2-standard-16"
  system_node_pool_taints       = []
  zone                          = var.zone
}

module "a3-ultragpu-pool" {
  source              = "github.com/GoogleCloudPlatform/cluster-toolkit.git//modules/compute/gke-node-pool?ref=e0c690b"
  additional_networks = concat([{ network = module.gke-a3-ultra-annuay-net-1.network_name, subnetwork = module.gke-a3-ultra-annuay-net-1.subnetwork_name, subnetwork_project = var.project_id, nic_type = "GVNIC", queue_count = null, network_ip = null, stack_type = null, access_config = [{ nat_ip = null, public_ptr_domain_name = null, network_tier = null }], ipv6_access_config = [], alias_ip_range = [] }], module.gke-a3-ultra-annuay-rdma-net.subnetwork_interfaces_gke)
  auto_upgrade        = true
  cluster_id          = module.a3-ultragpu-cluster.cluster_id
  disk_size_gb        = var.a3ultra_node_pool_disk_size_gb
  disk_type           = "hyperdisk-balanced"
  gke_version         = module.a3-ultragpu-cluster.gke_version
  guest_accelerator = [{
    count = 8
    gpu_driver_installation_config = {
      gpu_driver_version = "LATEST"
    }
    type = "nvidia-h200-141gb"
  }]
  internal_ghpc_module_id = "a3-ultragpu-pool"
  labels                  = var.labels
  machine_type            = "a3-ultragpu-8g"
  project_id              = var.project_id
  reservation_affinity = {
    consume_reservation_type = "SPECIFIC_RESERVATION"
    specific_reservations = [{
      name = var.extended_reservation
    }]
  }
  static_node_count = var.static_node_count
  zones             = [var.zone]
}

module "topology-aware-scheduler-install" {
  source     = "github.com/GoogleCloudPlatform/cluster-toolkit.git//community/modules/compute/gke-topology-scheduler?ref=e0c690b"
  cluster_id = module.a3-ultragpu-cluster.cluster_id
  project_id = var.project_id
}

module "workload-manager-install" {
  source = "github.com/GoogleCloudPlatform/cluster-toolkit.git//modules/management/kubectl-apply?ref=e0c690b"
  apply_manifests = [{
    source = var.nccl_installer_path
    }, {
    source = var.mglru_disable_path
  }]
  cluster_id = module.a3-ultragpu-cluster.cluster_id
  jobset = {
    install = true
    version = "v0.7.1"
  }
  kueue = {
    install = true
    version = "v0.9.1"
  }
  project_id = var.project_id
}

module "job-template" {
  source                   = "./modules/embedded/modules/compute/gke-job-template"
  allocatable_cpu_per_node = flatten([module.a3-ultragpu-pool.allocatable_cpu_per_node])
  command                  = ["nvidia-smi"]
  has_gpu                  = flatten([module.a3-ultragpu-pool.has_gpu])
  image                    = "nvidia/cuda:11.0.3-runtime-ubuntu20.04"
  labels                   = var.labels
  name                     = "run-nvidia-smi"
  node_count               = 2
  node_pool_name           = flatten([module.a3-ultragpu-pool.node_pool_name])
  tolerations              = flatten([module.a3-ultragpu-pool.tolerations])
}
