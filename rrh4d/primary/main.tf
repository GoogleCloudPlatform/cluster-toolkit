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
    bucket = "a4hsarthakag"
    prefix = "slurm-h4d/rrh4d/primary"
  }
}

module "h4d-slurm-net-0" {
  source          = "./modules/embedded/modules/network/vpc"
  deployment_name = var.deployment_name
  labels          = var.labels
  project_id      = var.project_id
  region          = var.region
}

module "h4d-rdma-net" {
  source                  = "./modules/embedded/modules/network/vpc"
  deployment_name         = var.deployment_name
  enable_cloud_nat        = false
  enable_cloud_router     = false
  enable_internal_traffic = false
  labels                  = var.labels
  mtu                     = 8896
  network_name            = "${var.deployment_name}-rdma-net-0"
  network_profile         = "https://www.googleapis.com/compute/beta/projects/${var.project_id}/global/networkProfiles/${var.zone}-vpc-falcon"
  network_routing_mode    = "REGIONAL"
  project_id              = var.project_id
  region                  = var.region
  subnetworks = [{
    region        = var.region
    subnet_ip     = var.rdma_net_range
    subnet_name   = "${var.deployment_name}-rdma-sub-0"
    subnet_region = var.region
  }]
}

module "homefs" {
  source               = "./modules/embedded/modules/file-system/filestore"
  deployment_name      = var.deployment_name
  filestore_share_name = "homeshare"
  filestore_tier       = "BASIC_SSD"
  labels               = var.labels
  local_mount          = "/home"
  network_id           = module.h4d-slurm-net-0.network_id
  project_id           = var.project_id
  region               = var.region
  size_gb              = 2560
  zone                 = var.zone
}

module "h4d_startup" {
  source          = "./modules/embedded/modules/scripts/startup-script"
  deployment_name = var.deployment_name
  labels          = var.labels
  local_ssd_filesystem = {
    fs_type     = "ext4"
    mountpoint  = "/mnt/lssd"
    permissions = "1777"
  }
  project_id                  = var.project_id
  region                      = var.region
  set_ofi_cloud_rdma_tunables = true
}

module "h4d_nodeset" {
  source                  = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-nodeset"
  additional_networks     = concat([{ network = null, subnetwork = module.h4d-rdma-net.subnetwork_self_link, subnetwork_project = var.project_id, nic_type = "IRDMA", queue_count = null, network_ip = null, stack_type = null, access_config = null, ipv6_access_config = [], alias_ip_range = [] }])
  allow_automatic_updates = false
  bandwidth_tier          = "gvnic_enabled"
  disk_type               = "hyperdisk-balanced"
  dws_flex = {
    enabled = var.h4d_dws_flex_enabled
  }
  enable_placement       = false
  enable_spot_vm         = var.h4d_enable_spot_vm
  instance_image         = var.instance_image
  labels                 = var.labels
  machine_type           = "h4d-highmem-192-lssd"
  name                   = "h4d_nodeset"
  node_count_dynamic_max = 0
  node_count_static      = var.h4d_cluster_size
  on_host_maintenance    = "TERMINATE"
  project_id             = var.project_id
  region                 = var.region
  reservation_name       = var.h4d_reservation_name
  startup_script         = module.h4d_startup.startup_script
  subnetwork_self_link   = module.h4d-slurm-net-0.subnetwork_self_link
  zone                   = var.zone
}

module "h4d_partition" {
  source     = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-partition"
  exclusive  = false
  is_default = true
  nodeset    = flatten([module.h4d_nodeset.nodeset])
  partition_conf = {
    ResumeTimeout  = 900
    SuspendTimeout = 600
  }
  partition_name = "h4d"
}

module "slurm_login" {
  source                  = "./modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-login"
  enable_login_public_ips = true
  instance_image          = var.instance_image
  labels                  = var.labels
  machine_type            = "n2-standard-4"
  name_prefix             = "slurm_login"
  project_id              = var.project_id
  region                  = var.region
  subnetwork_self_link    = module.h4d-slurm-net-0.subnetwork_self_link
  zone                    = var.zone
}

module "controller_startup" {
  source          = "./modules/embedded/modules/scripts/startup-script"
  deployment_name = var.deployment_name
  labels          = var.labels
  project_id      = var.project_id
  region          = var.region
  runners = [{
    content     = "#!/bin/bash\nSLURM_ROOT=/opt/apps/adm/slurm\nPARTITION_NAME=${module.h4d_partition.partitions[0].partition_name}\nmkdir -m 0755 -p \"$${SLURM_ROOT}/scripts\"\n\n# enable the use of password-free sudo within Slurm jobs on all compute nodes\n# feature is restricted to users with OS Admin Login IAM role\n# https://cloud.google.com/iam/docs/understanding-roles#compute.osAdminLogin\nmkdir -p \"$${SLURM_ROOT}/prolog_slurmd.d\"\nmkdir -p \"$${SLURM_ROOT}/epilog_slurmd.d\"\ncurl -s -o \"$${SLURM_ROOT}/scripts/sudo-oslogin\" \\\n    https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/master/tools/prologs-epilogs/sudo-oslogin\nchmod 0755 \"$${SLURM_ROOT}/scripts/sudo-oslogin\"\n\nln -s \"$${SLURM_ROOT}/scripts/sudo-oslogin\" \"$${SLURM_ROOT}/prolog_slurmd.d/sudo-oslogin.prolog_slurmd\"\nln -s \"$${SLURM_ROOT}/scripts/sudo-oslogin\" \"$${SLURM_ROOT}/epilog_slurmd.d/sudo-oslogin.epilog_slurmd\"\n\n# enable an RDMA health check that runs prior to any H4D job\nmkdir -p \"$${SLURM_ROOT}/partition-$${PARTITION_NAME}-prolog_slurmd.d\"\ncurl -s -o \"$${SLURM_ROOT}/partition-$${PARTITION_NAME}-prolog_slurmd.d/irdma_health_check.prolog_slurmd\" \\\n    https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/master/tools/prologs-epilogs/irdma_health_check\nchmod 0755 \"$${SLURM_ROOT}/partition-$${PARTITION_NAME}-prolog_slurmd.d/irdma_health_check.prolog_slurmd\"\n"
    destination = "stage_scripts.sh"
    type        = "shell"
  }]
}

module "slurm_controller" {
  source = "./modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-controller"
  cloud_parameters = {
    unkillable_step_timeout = 900
  }
  controller_startup_script     = module.controller_startup.startup_script
  deployment_name               = var.deployment_name
  enable_controller_public_ips  = true
  enable_external_prolog_epilog = true
  instance_image                = var.instance_image
  labels                        = var.labels
  login_nodes                   = flatten([module.slurm_login.login_nodes])
  network_storage               = flatten([module.homefs.network_storage])
  nodeset                       = flatten([module.h4d_partition.nodeset])
  nodeset_dyn                   = flatten([module.h4d_partition.nodeset_dyn])
  nodeset_tpu                   = flatten([module.h4d_partition.nodeset_tpu])
  partitions                    = flatten([module.h4d_partition.partitions])
  project_id                    = var.project_id
  region                        = var.region
  subnetwork_self_link          = module.h4d-slurm-net-0.subnetwork_self_link
  zone                          = var.zone
}
