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
  source          = "github.com/GoogleCloudPlatform/cluster-toolkit//modules/network/vpc?ref=v1.38.0&depth=1"
  deployment_name = var.deployment_name
  project_id      = var.project_id
  region          = var.region
}

module "controller_sa" {
  source          = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/project/service-account?ref=v1.37.0&depth=1"
  deployment_name = var.deployment_name
  name            = "controller"
  project_id      = var.project_id
  project_roles   = ["compute.instanceAdmin.v1", "iam.serviceAccountUser", "logging.logWriter", "monitoring.metricWriter", "pubsub.admin", "storage.objectViewer"]
}

module "login_sa" {
  source          = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/project/service-account?ref=v1.38.0&depth=1"
  deployment_name = var.deployment_name
  name            = "login"
  project_id      = var.project_id
  project_roles   = ["logging.logWriter", "monitoring.metricWriter", "storage.objectViewer"]
}

module "compute_sa" {
  source          = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/project/service-account?ref=v1.38.0&depth=1"
  deployment_name = var.deployment_name
  name            = "compute"
  project_id      = var.project_id
  project_roles   = ["logging.logWriter", "monitoring.metricWriter", "storage.objectCreator"]
}

module "homefs" {
  source          = "github.com/GoogleCloudPlatform/cluster-toolkit//modules/file-system/filestore?ref=v1.37.0&depth=1"
  deployment_name = var.deployment_name
  labels          = var.labels
  local_mount     = "/home"
  network_id      = module.network.network_id
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
}

module "projectsfs" {
  source          = "github.com/GoogleCloudPlatform/cluster-toolkit//modules/file-system/filestore?ref=v1.38.0&depth=1"
  deployment_name = var.deployment_name
  labels          = var.labels
  local_mount     = "/projects"
  network_id      = module.network.network_id
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
}

module "scratchfs" {
  source               = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/file-system/DDN-EXAScaler?ref=v1.38.0&depth=1"
  labels               = var.labels
  local_mount          = "/scratch"
  network_self_link    = module.network.network_self_link
  project_id           = var.project_id
  subnetwork_address   = module.network.subnetwork_address
  subnetwork_self_link = module.network.subnetwork_self_link
  zone                 = var.zone
}

module "n2_nodeset" {
  source                  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset?ref=v1.38.0&depth=1"
  allow_automatic_updates = false
  enable_placement        = false
  instance_image          = var.slurm_image
  instance_image_custom   = var.instance_image_custom
  labels                  = var.labels
  machine_type            = "n2-standard-2"
  name                    = "n2_nodeset"
  node_count_dynamic_max  = 4
  project_id              = var.project_id
  region                  = var.region
  service_account_email   = module.compute_sa.service_account_email
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "n2_partition" {
  source     = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition?ref=v1.38.0&depth=1"
  exclusive  = false
  is_default = true
  nodeset    = flatten([module.n2_nodeset.nodeset])
  partition_conf = {
    SuspendTime = 300
  }
  partition_name = "n2"
}

module "c2_nodeset" {
  source                  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset?ref=v1.38.0&depth=1"
  allow_automatic_updates = false
  bandwidth_tier          = "tier_1_enabled"
  disk_size_gb            = 100
  disk_type               = "pd-ssd"
  enable_placement        = true
  instance_image          = var.slurm_image
  instance_image_custom   = var.instance_image_custom
  labels                  = var.labels
  machine_type            = "c2-standard-60"
  name                    = "c2_nodeset"
  node_count_dynamic_max  = 20
  project_id              = var.project_id
  region                  = var.region
  service_account_email   = module.compute_sa.service_account_email
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "c2_partition" {
  source         = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition?ref=v1.38.0&depth=1"
  exclusive      = true
  nodeset        = flatten([module.c2_nodeset.nodeset])
  partition_name = "c2"
}

module "c2d_nodeset" {
  source                  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset?ref=v1.38.0&depth=1"
  allow_automatic_updates = false
  bandwidth_tier          = "tier_1_enabled"
  disk_size_gb            = 100
  disk_type               = "pd-ssd"
  instance_image          = var.slurm_image
  instance_image_custom   = var.instance_image_custom
  labels                  = var.labels
  machine_type            = "c2d-standard-112"
  name                    = "c2d_nodeset"
  node_count_dynamic_max  = 20
  project_id              = var.project_id
  region                  = var.region
  service_account_email   = module.compute_sa.service_account_email
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "c2d_partition" {
  source         = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition?ref=v1.38.0&depth=1"
  nodeset        = flatten([module.c2d_nodeset.nodeset])
  partition_name = "c2d"
}

module "c3_nodeset" {
  source                  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset?ref=v1.38.0&depth=1"
  allow_automatic_updates = false
  bandwidth_tier          = "tier_1_enabled"
  disk_size_gb            = 100
  disk_type               = "pd-ssd"
  instance_image          = var.slurm_image
  instance_image_custom   = var.instance_image_custom
  labels                  = var.labels
  machine_type            = "c3-highcpu-176"
  name                    = "c3_nodeset"
  node_count_dynamic_max  = 20
  project_id              = var.project_id
  region                  = var.region
  service_account_email   = module.compute_sa.service_account_email
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "c3_partition" {
  source         = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition?ref=v1.38.0&depth=1"
  nodeset        = flatten([module.c3_nodeset.nodeset])
  partition_name = "c3"
}

module "a2_8_nodeset" {
  source                  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset?ref=v1.38.0&depth=1"
  allow_automatic_updates = false
  bandwidth_tier          = "gvnic_enabled"
  disk_size_gb            = 100
  disk_type               = "pd-ssd"
  instance_image          = var.slurm_image
  instance_image_custom   = var.instance_image_custom
  labels                  = var.labels
  machine_type            = "a2-ultragpu-8g"
  name                    = "a2_8_nodeset"
  node_conf = {
    CoresPerSocket  = 24
    SocketsPerBoard = 2
  }
  node_count_dynamic_max = 16
  project_id             = var.project_id
  region                 = var.region
  service_account_email  = module.compute_sa.service_account_email
  subnetwork_self_link   = module.network.subnetwork_self_link
  zone                   = var.zone
  zones                  = var.gpu_zones
}

module "a2_8_partition" {
  source  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition?ref=v1.38.0&depth=1"
  nodeset = flatten([module.a2_8_nodeset.nodeset])
  partition_conf = {
    DefMemPerCPU = null
    DefMemPerGPU = 160000
  }
  partition_name = "a208"
}

module "a2_16_nodeset" {
  source                  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset?ref=v1.38.0&depth=1"
  allow_automatic_updates = false
  bandwidth_tier          = "gvnic_enabled"
  disk_size_gb            = 100
  disk_type               = "pd-ssd"
  instance_image          = var.slurm_image
  instance_image_custom   = var.instance_image_custom
  labels                  = var.labels
  machine_type            = "a2-megagpu-16g"
  name                    = "a2_16_nodeset"
  node_conf = {
    CoresPerSocket  = 24
    SocketsPerBoard = 2
  }
  node_count_dynamic_max = 16
  project_id             = var.project_id
  region                 = var.region
  service_account_email  = module.compute_sa.service_account_email
  subnetwork_self_link   = module.network.subnetwork_self_link
  zone                   = var.zone
  zones                  = var.gpu_zones
}

module "a2_16_partition" {
  source  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition?ref=v1.38.0&depth=1"
  nodeset = flatten([module.a2_16_nodeset.nodeset])
  partition_conf = {
    DefMemPerCPU = null
    DefMemPerGPU = 160000
  }
  partition_name = "a216"
}

module "h3_nodeset" {
  source                  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset?ref=v1.38.0&depth=1"
  allow_automatic_updates = false
  bandwidth_tier          = "gvnic_enabled"
  disk_size_gb            = 100
  disk_type               = "pd-balanced"
  instance_image          = var.slurm_image
  instance_image_custom   = var.instance_image_custom
  labels                  = var.labels
  machine_type            = "h3-standard-88"
  name                    = "h3_nodeset"
  node_count_dynamic_max  = 16
  project_id              = var.project_id
  region                  = var.region
  service_account_email   = module.compute_sa.service_account_email
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "h3_partition" {
  source         = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition?ref=v1.38.0&depth=1"
  nodeset        = flatten([module.h3_nodeset.nodeset])
  partition_name = "h3"
}

module "slurm_login" {
  source                  = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/scheduler/schedmd-slurm-gcp-v6-login?ref=v1.38.0&depth=1"
  enable_login_public_ips = true
  instance_image          = var.slurm_image
  instance_image_custom   = var.instance_image_custom
  labels                  = var.labels
  machine_type            = "n2-standard-4"
  name_prefix             = "slurm_login"
  project_id              = var.project_id
  region                  = var.region
  service_account_email   = module.login_sa.service_account_email
  subnetwork_self_link    = module.network.subnetwork_self_link
  zone                    = var.zone
}

module "slurm_controller" {
  source = "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/scheduler/schedmd-slurm-gcp-v6-controller?ref=v1.38.0&depth=1"
  cloud_parameters = {
    no_comma_params = false
    resume_rate     = 0
    resume_timeout  = 600
    suspend_rate    = 0
    suspend_timeout = 600
  }
  deployment_name              = var.deployment_name
  enable_controller_public_ips = true
  instance_image               = var.slurm_image
  instance_image_custom        = var.instance_image_custom
  labels                       = var.labels
  login_nodes                  = flatten([module.slurm_login.login_nodes])
  network_storage              = flatten([module.scratchfs.network_storage, flatten([module.projectsfs.network_storage, flatten([module.homefs.network_storage])])])
  nodeset                      = flatten([module.h3_partition.nodeset, flatten([module.a2_16_partition.nodeset, flatten([module.a2_8_partition.nodeset, flatten([module.c3_partition.nodeset, flatten([module.c2d_partition.nodeset, flatten([module.c2_partition.nodeset, flatten([module.n2_partition.nodeset])])])])])])])
  nodeset_dyn                  = flatten([module.h3_partition.nodeset_dyn, flatten([module.a2_16_partition.nodeset_dyn, flatten([module.a2_8_partition.nodeset_dyn, flatten([module.c3_partition.nodeset_dyn, flatten([module.c2d_partition.nodeset_dyn, flatten([module.c2_partition.nodeset_dyn, flatten([module.n2_partition.nodeset_dyn])])])])])])])
  nodeset_tpu                  = flatten([module.h3_partition.nodeset_tpu, flatten([module.a2_16_partition.nodeset_tpu, flatten([module.a2_8_partition.nodeset_tpu, flatten([module.c3_partition.nodeset_tpu, flatten([module.c2d_partition.nodeset_tpu, flatten([module.c2_partition.nodeset_tpu, flatten([module.n2_partition.nodeset_tpu])])])])])])])
  partitions                   = flatten([module.h3_partition.partitions, flatten([module.a2_16_partition.partitions, flatten([module.a2_8_partition.partitions, flatten([module.c3_partition.partitions, flatten([module.c2d_partition.partitions, flatten([module.c2_partition.partitions, flatten([module.n2_partition.partitions])])])])])])])
  project_id                   = var.project_id
  region                       = var.region
  service_account_email        = module.controller_sa.service_account_email
  subnetwork_self_link         = module.network.subnetwork_self_link
  zone                         = var.zone
}

module "hpc_dashboard" {
  source          = "github.com/GoogleCloudPlatform/cluster-toolkit//modules/monitoring/dashboard?ref=v1.38.0&depth=1"
  deployment_name = var.deployment_name
  labels          = var.labels
  project_id      = var.project_id
}
