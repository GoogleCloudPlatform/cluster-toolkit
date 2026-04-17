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
    prefix = "a4xhigh-slurm/sarthakaga4x/cluster"
  }
}

module "a4x_nodeset" {
  source               = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-nodeset"
  accelerator_topology = "1x72"
  additional_networks  = concat([{ network = null, subnetwork = var.subnetwork_self_link_a4x-slurm-net-1, subnetwork_project = var.project_id, nic_type = "GVNIC", queue_count = null, network_ip = "", stack_type = null, access_config = [], ipv6_access_config = [], alias_ip_range = [] }], var.subnetwork_interfaces_a4x-slurm-rdma-net)
  advanced_machine_features = {
    threads_per_core = null
  }
  bandwidth_tier        = "gvnic_enabled"
  disk_size_gb          = var.disk_size_gb
  disk_type             = "hyperdisk-balanced"
  enable_placement      = true
  enable_public_ips     = true
  instance_image        = var.instance_image
  instance_image_custom = true
  labels                = var.labels
  machine_type          = "a4x-highgpu-4g"
  name                  = "a4x_nodeset"
  node_conf = {
    CoresPerSocket  = 70
    SocketsPerBoard = 2
    ThreadsPerCore  = 1
  }
  node_count_dynamic_max = 0
  node_count_static      = var.a4x_cluster_size
  on_host_maintenance    = "TERMINATE"
  project_id             = var.project_id
  region                 = var.region
  reservation_name       = var.a4x_reservation_name
  startup_script         = var.startup_script_a4x_startup
  subnetwork_self_link   = var.subnetwork_self_link_a4x-slurm-net-0
  zone                   = var.zone
}

module "a4x_partition" {
  source     = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-partition"
  exclusive  = false
  is_default = true
  nodeset    = flatten([module.a4x_nodeset.nodeset])
  partition_conf = {
    OverSubscribe  = "EXCLUSIVE"
    ResumeTimeout  = 3600
    SuspendTimeout = 600
  }
  partition_name = "a4x"
}

module "slurm_login" {
  source                  = "./modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-login"
  disk_size_gb            = var.disk_size_gb
  disk_type               = "hyperdisk-balanced"
  enable_login_public_ips = true
  instance_image          = var.instance_image
  instance_image_custom   = true
  labels                  = var.labels
  machine_type            = "c4a-standard-4"
  name_prefix             = "slurm_login"
  project_id              = var.project_id
  region                  = var.region
  subnetwork_self_link    = var.subnetwork_self_link_a4x-slurm-net-0
  zone                    = var.zone
}

module "controller_startup" {
  source          = "./modules/embedded/modules/scripts/startup-script"
  deployment_name = var.deployment_name
  labels          = var.labels
  project_id      = var.project_id
  region          = var.region
  runners = [{
    content     = "#!/bin/bash\nSLURM_ROOT=/opt/apps/adm/slurm\nPARTITION_NAME=${module.a4x_partition.partitions[0].partition_name}\nmkdir -m 0755 -p \"$${SLURM_ROOT}/scripts\"\n# enable a GPU health check that runs at the completion of all jobs on A4X nodes\nmkdir -p \"$${SLURM_ROOT}/partition-$${PARTITION_NAME}-epilog_slurmd.d\"\nln -s \"/slurm/scripts/tools/gpu-test\" \"$${SLURM_ROOT}/partition-$${PARTITION_NAME}-epilog_slurmd.d/gpu-test.epilog_slurmd\"\n# enable the use of password-free sudo within Slurm jobs on all compute nodes\n# feature is restricted to users with OS Admin Login IAM role\n# https://cloud.google.com/iam/docs/understanding-roles#compute.osAdminLogin\nmkdir -p \"$${SLURM_ROOT}/prolog_slurmd.d\"\nmkdir -p \"$${SLURM_ROOT}/epilog_slurmd.d\"\ncurl -s -o \"$${SLURM_ROOT}/scripts/sudo-oslogin\" \\\n    https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/master/tools/prologs-epilogs/sudo-oslogin\nchmod 0755 \"$${SLURM_ROOT}/scripts/sudo-oslogin\"\nln -s \"$${SLURM_ROOT}/scripts/sudo-oslogin\" \"$${SLURM_ROOT}/prolog_slurmd.d/sudo-oslogin.prolog_slurmd\"\nln -s \"$${SLURM_ROOT}/scripts/sudo-oslogin\" \"$${SLURM_ROOT}/epilog_slurmd.d/sudo-oslogin.epilog_slurmd\"\ncurl -s -o \"$${SLURM_ROOT}/scripts/imex_prolog\" \\\n    https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/master/tools/prologs-epilogs/imex_prolog\nchmod 0755 \"$${SLURM_ROOT}/scripts/imex_prolog\"\nln -s \"$${SLURM_ROOT}/scripts/imex_prolog\" \"$${SLURM_ROOT}/prolog_slurmd.d/imex_prolog.prolog_slurmd\"\ncurl -s -o \"$${SLURM_ROOT}/scripts/imex_epilog\" \\\n    https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/master/tools/prologs-epilogs/imex_epilog\nchmod 0755 \"$${SLURM_ROOT}/scripts/imex_epilog\"\nln -s \"$${SLURM_ROOT}/scripts/imex_epilog\" \"$${SLURM_ROOT}/epilog_slurmd.d/imex_epilog.epilog_slurmd\"\n"
    destination = "stage_scripts.sh"
    type        = "shell"
    }, {
    destination = "/opt/apps/system_benchmarks/run-nccl-tests-via-ramble.sh"
    source      = "${var.benchmark_dir}/run-nccl-tests-via-ramble.sh"
    type        = "data"
    }, {
    destination = "/opt/apps/system_benchmarks/README.md"
    source      = "${var.benchmark_dir}/README.md"
    type        = "data"
  }]
}

module "slurm_controller" {
  source = "./modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-controller"
  cloud_parameters = {
    prolog_flags = "Alloc,DeferBatch,NoHold"
    switch_type  = "switch/nvidia_imex"
  }
  controller_startup_script    = module.controller_startup.startup_script
  controller_state_disk        = null
  deployment_name              = var.deployment_name
  disk_size_gb                 = var.disk_size_gb
  disk_type                    = "hyperdisk-balanced"
  enable_controller_public_ips = true
  instance_image               = var.instance_image
  instance_image_custom        = true
  labels                       = var.labels
  login_nodes                  = flatten([module.slurm_login.login_nodes])
  machine_type                 = "c4a-highcpu-16"
  network_storage              = flatten([var.network_storage_gcs_bucket, flatten([var.network_storage_homefs])])
  nodeset                      = flatten([module.a4x_partition.nodeset])
  nodeset_dyn                  = flatten([module.a4x_partition.nodeset_dyn])
  nodeset_tpu                  = flatten([module.a4x_partition.nodeset_tpu])
  partitions                   = flatten([module.a4x_partition.partitions])
  project_id                   = var.project_id
  region                       = var.region
  subnetwork_self_link         = var.subnetwork_self_link_a4x-slurm-net-0
  zone                         = var.zone
}
