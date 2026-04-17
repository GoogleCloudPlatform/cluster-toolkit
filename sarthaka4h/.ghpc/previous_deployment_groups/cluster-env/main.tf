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
    bucket = "sarthakag-test"
    prefix = "a4high-slurm/sarthaka4h/cluster-env"
  }
}

module "a4high-slurm-net-0" {
  source                  = "./modules/embedded/modules/network/vpc"
  deployment_name         = var.deployment_name
  enable_internal_traffic = false
  firewall_rules = [{
    allow = [{
      protocol = "tcp"
      }, {
      protocol = "udp"
      }, {
      protocol = "icmp"
    }]
    name   = "${var.base_network_name}-internal-0"
    ranges = [var.net0_range]
  }]
  labels       = var.labels
  mtu          = 8896
  network_name = "${var.base_network_name}-net-0"
  project_id   = var.project_id
  region       = var.region
  subnetworks = [{
    subnet_ip     = var.net0_range
    subnet_name   = "${var.base_network_name}-sub-0"
    subnet_region = var.region
  }]
}

module "a4high-slurm-net-1" {
  source                  = "./modules/embedded/modules/network/vpc"
  deployment_name         = var.deployment_name
  enable_internal_traffic = false
  firewall_rules = [{
    allow = [{
      protocol = "tcp"
      }, {
      protocol = "udp"
      }, {
      protocol = "icmp"
    }]
    name   = "${var.base_network_name}-internal-1"
    ranges = [var.net1_range]
  }]
  labels       = var.labels
  mtu          = 8896
  network_name = "${var.base_network_name}-net-1"
  project_id   = var.project_id
  region       = var.region
  subnetworks = [{
    subnet_ip     = var.net1_range
    subnet_name   = "${var.base_network_name}-sub-1"
    subnet_region = var.region
  }]
}

module "a4high-slurm-rdma-net" {
  source               = "./modules/embedded/modules/network/gpu-rdma-vpc"
  deployment_name      = var.deployment_name
  network_name         = "${var.base_network_name}-rdma-net"
  network_profile      = "https://www.googleapis.com/compute/beta/projects/${var.project_id}/global/networkProfiles/${var.zone}-vpc-roce"
  network_routing_mode = "REGIONAL"
  project_id           = var.project_id
  region               = var.region
  subnetworks_template = {
    count       = 8
    ip_range    = var.rdma_net_range
    name_prefix = "${var.base_network_name}-mrdma-sub"
    region      = var.region
  }
}

module "homefs" {
  source = "./modules/embedded/modules/file-system/filestore"
  deletion_protection = {
    enabled = true
    reason  = "Avoid data loss"
  }
  deployment_name   = var.deployment_name
  filestore_tier    = "HIGH_SCALE_SSD"
  labels            = var.labels
  local_mount       = "/home"
  network_id        = module.a4high-slurm-net-0.network_id
  project_id        = var.project_id
  region            = var.region
  reserved_ip_range = var.filestore_ip_range
  size_gb           = 10240
  zone              = var.zone
}

module "gcs_bucket" {
  source                        = "./modules/embedded/modules/file-system/cloud-storage-bucket"
  deployment_name               = var.deployment_name
  enable_hierarchical_namespace = true
  labels                        = var.labels
  local_mount                   = "/gcs"
  mount_options                 = "implicit-dirs,metadata-cache-negative-ttl-secs=0,metadata-cache-ttl-secs=-1,stat-cache-max-size-mb=-1,type-cache-max-size-mb=-1,enable-streaming-writes=true,dir-mode=777,file-mode=777,allow_other"
  project_id                    = var.project_id
  random_suffix                 = true
  region                        = var.region
}

module "gcs_checkpoints" {
  source        = "./modules/embedded/modules/file-system/pre-existing-network-storage"
  fs_type       = "gcsfuse"
  local_mount   = "/gcs-checkpoints"
  mount_options = "implicit-dirs,metadata-cache-negative-ttl-secs=0,metadata-cache-ttl-secs=-1,stat-cache-max-size-mb=-1,type-cache-max-size-mb=-1,file-cache-max-size-mb=-1,file-cache-cache-file-for-range-read=true,file-cache-enable-parallel-downloads=true,cache-dir=/mnt/localssd,enable-streaming-writes=true,dir-mode=777,file-mode=777,allow_other"
  remote_mount  = module.gcs_bucket.gcs_bucket_name
}

module "gcs_training_data" {
  source        = "./modules/embedded/modules/file-system/pre-existing-network-storage"
  fs_type       = "gcsfuse"
  local_mount   = "/gcs-training-data"
  mount_options = "implicit-dirs,metadata-cache-negative-ttl-secs=0,metadata-cache-ttl-secs=-1,stat-cache-max-size-mb=-1,type-cache-max-size-mb=-1,enable-streaming-writes=true,dir-mode=777,file-mode=777,allow_other"
  remote_mount  = module.gcs_bucket.gcs_bucket_name
}

module "gcs_model_serving" {
  source        = "./modules/embedded/modules/file-system/pre-existing-network-storage"
  fs_type       = "gcsfuse"
  local_mount   = "/gcs-model-serving"
  mount_options = "implicit-dirs,cache-dir=/mnt/localssd,metadata-cache-negative-ttl-secs=0,metadata-cache-ttl-secs=-1,stat-cache-max-size-mb=-1,type-cache-max-size-mb=-1,file-cache-max-size-mb=-1,file-cache-cache-file-for-range-read=true,file-cache-enable-parallel-downloads=true,dir-mode=777,file-mode=777,allow_other"
  remote_mount  = module.gcs_bucket.gcs_bucket_name
}
