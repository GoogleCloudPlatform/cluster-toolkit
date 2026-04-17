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
    bucket = "sarthakag-ctk"
    prefix = "gke-a4/chsa4gke/primary"
  }
}

module "gke-a4-net-0" {
  source          = "./modules/embedded/modules/network/vpc"
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
    name   = "${var.deployment_name}-internal-0"
    ranges = ["192.168.0.0/16"]
  }]
  ips_per_nat  = 6
  labels       = var.labels
  network_name = "${var.deployment_name}-net-0"
  project_id   = var.project_id
  region       = var.region
  secondary_ranges_list = [{
    ranges = [{
      ip_cidr_range = "10.4.0.0/14"
      range_name    = "pods"
      }, {
      ip_cidr_range = "10.0.32.0/20"
      range_name    = "services"
    }]
    subnetwork_name = "${var.deployment_name}-sub-0"
  }]
  subnetworks = [{
    subnet_ip     = "192.168.0.0/18"
    subnet_name   = "${var.deployment_name}-sub-0"
    subnet_region = var.region
  }]
}

module "gke-a4-net-1" {
  source          = "./modules/embedded/modules/network/vpc"
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
    name   = "${var.deployment_name}-internal-1"
    ranges = ["192.168.0.0/16"]
  }]
  ips_per_nat  = 6
  labels       = var.labels
  network_name = "${var.deployment_name}-net-1"
  project_id   = var.project_id
  region       = var.region
  subnetworks = [{
    subnet_ip     = "192.168.64.0/18"
    subnet_name   = "${var.deployment_name}-sub-1"
    subnet_region = var.region
  }]
}

module "gke-a4-rdma-net" {
  source               = "./modules/embedded/modules/network/gpu-rdma-vpc"
  deployment_name      = var.deployment_name
  network_name         = "${var.deployment_name}-rdma-net"
  network_profile      = "https://www.googleapis.com/compute/beta/projects/${var.project_id}/global/networkProfiles/${var.zone}-vpc-roce"
  network_routing_mode = "REGIONAL"
  project_id           = var.project_id
  region               = var.region
  subnetworks_template = {
    count       = 8
    ip_range    = "192.168.128.0/18"
    name_prefix = "${var.deployment_name}-rdma-sub"
    region      = var.region
  }
}

module "node_pool_service_account" {
  source          = "./modules/embedded/modules/project/service-account"
  deployment_name = var.deployment_name
  name            = "gke-np-sa"
  project_id      = var.project_id
  project_roles   = ["logging.logWriter", "monitoring.metricWriter", "monitoring.viewer", "stackdriver.resourceMetadata.writer", "storage.objectAdmin", "artifactregistry.reader"]
}

module "workload_service_account" {
  source          = "./modules/embedded/modules/project/service-account"
  deployment_name = var.deployment_name
  name            = "gke-wl-sa"
  project_id      = var.project_id
  project_roles   = ["logging.logWriter", "monitoring.metricWriter", "monitoring.viewer", "stackdriver.resourceMetadata.writer", "storage.objectAdmin", "artifactregistry.reader", "container.admin"]
}

module "training_bucket" {
  source                        = "./modules/embedded/modules/file-system/cloud-storage-bucket"
  deployment_name               = var.deployment_name
  enable_hierarchical_namespace = true
  force_destroy                 = false
  labels                        = var.labels
  local_mount                   = "/training-data"
  name_prefix                   = "training"
  project_id                    = var.project_id
  random_suffix                 = true
  region                        = var.region
}

module "checkpoint_bucket" {
  source                        = "./modules/embedded/modules/file-system/cloud-storage-bucket"
  deployment_name               = var.deployment_name
  enable_hierarchical_namespace = true
  force_destroy                 = false
  labels                        = var.labels
  local_mount                   = "/checkpoint-data"
  name_prefix                   = "checkpoint"
  project_id                    = var.project_id
  random_suffix                 = true
  region                        = var.region
}

module "a4-cluster" {
  source                         = "./modules/embedded/modules/scheduler/gke-cluster"
  additional_networks            = concat([{ network = module.gke-a4-net-1.network_name, subnetwork = module.gke-a4-net-1.subnetwork_name, subnetwork_project = var.project_id, nic_type = "GVNIC", queue_count = null, network_ip = null, stack_type = null, access_config = [{ nat_ip = null, public_ptr_domain_name = null, network_tier = null }], ipv6_access_config = [], alias_ip_range = [] }], module.gke-a4-rdma-net.subnetwork_interfaces_gke)
  configure_workload_identity_sa = true
  deployment_name                = var.deployment_name
  enable_dcgm_monitoring         = true
  enable_gcsfuse_csi             = true
  enable_managed_lustre_csi      = true
  enable_private_endpoint        = false
  labels                         = var.labels
  maintenance_exclusions = [{
    exclusion_end_time_behavior = "UNTIL_END_OF_SUPPORT"
    exclusion_scope             = "NO_MINOR_OR_NODE_UPGRADES"
    name                        = "no-minor-or-node-upgrades-indefinite"
    start_time                  = "2025-12-01T00:00:00Z"
  }]
  master_authorized_networks = [{
    cidr_block   = var.authorized_cidr
    display_name = "kubectl-access-network"
  }]
  network_id                    = module.gke-a4-net-0.network_id
  project_id                    = var.project_id
  region                        = var.region
  release_channel               = "REGULAR"
  service_account_email         = module.workload_service_account.service_account_email
  subnetwork_self_link          = module.gke-a4-net-0.subnetwork_self_link
  system_node_pool_disk_size_gb = var.system_node_pool_disk_size_gb
  system_node_pool_machine_type = "e2-standard-16"
  system_node_pool_taints       = []
  version_prefix                = var.version_prefix
  zone                          = var.zone
}

module "a4-pool" {
  source              = "./modules/embedded/modules/compute/gke-node-pool"
  additional_networks = concat([{ network = module.gke-a4-net-1.network_name, subnetwork = module.gke-a4-net-1.subnetwork_name, subnetwork_project = var.project_id, nic_type = "GVNIC", queue_count = null, network_ip = null, stack_type = null, access_config = [{ nat_ip = null, public_ptr_domain_name = null, network_tier = null }], ipv6_access_config = [], alias_ip_range = [] }], module.gke-a4-rdma-net.subnetwork_interfaces_gke)
  auto_upgrade        = true
  cluster_id          = module.a4-cluster.cluster_id
  disk_size_gb        = var.a4_node_pool_disk_size_gb
  gke_version         = module.a4-cluster.gke_version
  guest_accelerator = [{
    count = 8
    type  = var.accelerator_type
  }]
  internal_ghpc_module_id = "a4-pool"
  labels                  = var.labels
  machine_type            = "a4-highgpu-8g"
  placement_policy = {
    type = "COMPACT"
  }
  project_id            = var.project_id
  service_account_email = module.node_pool_service_account.service_account_email
  spot                  = true
  static_node_count     = var.static_node_count
  zones                 = [var.zone]
}

module "workload-manager-install" {
  source = "./modules/embedded/modules/management/kubectl-apply"
  apply_manifests = [{
    enable = var.enable_periodic_health_checks
    source = var.permissions_file_staged_path
    template_vars = {
      deployment_name = var.deployment_name
      project_id      = var.project_id
    }
    }, {
    enable = var.enable_periodic_health_checks
    source = var.chs_pvc_rendered_path
    template_vars = {
      access_mode        = "ReadWriteOnce"
      capacity           = "1Gi"
      pvc_name           = var.chs_pvc_claim_name
      storage_class_name = "standard-rwo"
    }
    }, {
    enable = var.enable_periodic_health_checks
    source = var.chs_cronjob_rendered_path
    template_vars = {
      cronjob_schedule = var.health_check_schedule
      deployment_name  = var.deployment_name
      gcs_bucket       = var.chs_output_bucket_name
      gcs_pvc          = var.chs_pvc_claim_name
      machine_type     = "a4-highgpu-8g"
      project_id       = var.project_id
      region           = var.region
    }
  }]
  cluster_id = module.a4-cluster.cluster_id
  gib = {
    install = true
    path    = var.gib_installer_path
    template_vars = {
      accelerator_count = 8
      version           = "v1.1.0"
    }
  }
  gke_cluster_exists = module.a4-cluster.gke_cluster_exists
  jobset = {
    install = true
  }
  kueue = {
    config_path = var.kueue_configuration_path
    config_template_vars = {
      accelerator_type = var.accelerator_type
      num_gpus         = module.a4-pool.static_gpu_count
    }
    install = true
    wait    = false
  }
  project_id = var.project_id
}

module "job-template" {
  source                   = "./modules/embedded/modules/compute/gke-job-template"
  allocatable_cpu_per_node = flatten([module.a4-pool.allocatable_cpu_per_node])
  allocatable_gpu_per_node = flatten([module.a4-pool.allocatable_gpu_per_node])
  command                  = ["nvidia-smi"]
  has_gpu                  = flatten([module.a4-pool.has_gpu])
  image                    = "nvidia/cuda:11.0.3-runtime-ubuntu20.04"
  k8s_service_account_name = "workload-identity-k8s-sa"
  labels                   = var.labels
  name                     = "run-nvidia-smi"
  node_count               = var.static_node_count
  node_pool_names          = flatten([module.a4-pool.node_pool_names])
  tolerations              = flatten([module.a4-pool.tolerations])
  tpu_accelerator_type     = flatten([module.a4-pool.tpu_accelerator_type])
  tpu_chips_per_node       = flatten([module.a4-pool.tpu_chips_per_node])
  tpu_topology             = flatten([module.a4-pool.tpu_topology])
}

module "gcs-training" {
  source        = "./modules/embedded/modules/file-system/pre-existing-network-storage"
  fs_type       = "gcsfuse"
  local_mount   = "/training-data"
  mount_options = "implicit-dirs,metadata-cache:ttl-secs:-1,metadata-cache:stat-cache-max-size-mb:-1,metadata-cache:type-cache-max-size-mb:-1,file-cache:max-size-mb:-1,file-cache:cache-file-for-range-read:true"
  remote_mount  = module.training_bucket.gcs_bucket_name
}

module "gcs-checkpointing" {
  source        = "./modules/embedded/modules/file-system/pre-existing-network-storage"
  fs_type       = "gcsfuse"
  local_mount   = "/checkpoint-data"
  mount_options = "implicit-dirs,metadata-cache:ttl-secs:-1,metadata-cache:stat-cache-max-size-mb:-1,metadata-cache:type-cache-max-size-mb:-1,file-cache:max-size-mb:-1,file-cache:cache-file-for-range-read:true,file-cache:enable-parallel-downloads:true,rename-dir-limit=200000"
  remote_mount  = module.checkpoint_bucket.gcs_bucket_name
}

module "training-pv" {
  source          = "./modules/embedded/modules/file-system/gke-persistent-volume"
  capacity_gib    = 1000000
  cluster_id      = module.a4-cluster.cluster_id
  gcs_bucket_name = module.training_bucket.gcs_bucket_name
  labels          = var.labels
  network_storage = module.gcs-training.network_storage
}

module "checkpointing-pv" {
  source          = "./modules/embedded/modules/file-system/gke-persistent-volume"
  capacity_gib    = 1000000
  cluster_id      = module.a4-cluster.cluster_id
  gcs_bucket_name = module.checkpoint_bucket.gcs_bucket_name
  labels          = var.labels
  network_storage = module.gcs-checkpointing.network_storage
}

module "fio-bench-job-template" {
  source                   = "./modules/embedded/modules/compute/gke-job-template"
  allocatable_cpu_per_node = flatten([module.a4-pool.allocatable_cpu_per_node])
  allocatable_gpu_per_node = flatten([module.a4-pool.allocatable_gpu_per_node])
  command                  = ["bash", "-c", "set -eux\nexport DEBIAN_FRONTEND=noninteractive\n\n# Install fio\napt update -y && apt install -y fio\n\n# Use a tag to create a unique path for tests\nTAG=`date +%s`\n\n# Verify mountpoints\ndf -h\nmountpoint /scratch-data\nmountpoint /checkpoint-data\nmountpoint /training-data\n\n# Create temporary directory for fio benchmarks\nmkdir -p /{scratch,training,checkpoint}-data/fio-benchmarks-$${TAG}\n\n# The following will take roughly 10 minutes to complete\n\n# Perform scratch data write performance test\nfio --ioengine=libaio --filesize=10G --ramp_time=2s --runtime=1m \\\n  --numjobs=32 --create_serialize=0 --direct=1 --verify=0 \\\n  --randrepeat=0 --group_reporting --directory=/scratch-data/fio-benchmarks-$${TAG} \\\n  --name=scratch --blocksize=100m --iodepth=64 --readwrite=write\n\n# Perform training data reading performance test\nfio --ioengine=libaio --filesize=1G --ramp_time=2s --runtime=1m \\\n  --numjobs=32 --create_serialize=0 --direct=1 --verify=0 \\\n  --randrepeat=0 --group_reporting --directory=/training-data/fio-benchmarks-$${TAG} \\\n  --name=training --blocksize=1m --iodepth=64 --readwrite=randread\n\n# Perform checkpoint data writing performance test\nfio --ioengine=libaio --filesize=10G --ramp_time=2s --runtime=1m \\\n  --numjobs=32 --create_serialize=0 --direct=1 --verify=0 \\\n  --randrepeat=0 --group_reporting --directory=/checkpoint-data/fio-benchmarks-$${TAG} \\\n  --name=checkpoint --blocksize=100m --iodepth=64 --readwrite=write\n\n# Perform checkpoint data reading performance test\nfio --ioengine=libaio --filesize=10G --ramp_time=2s --runtime=1m \\\n  --numjobs=32 --create_serialize=0 --direct=1 --verify=0 \\\n  --randrepeat=0 --group_reporting --directory=/checkpoint-data/fio-benchmarks-$${TAG} \\\n  --name=checkpoint --blocksize=100m --iodepth=64 --readwrite=read\n\n# Clean up temporary directories for fio benchmarks\nrm -rf /{scratch,training,checkpoint}-data/fio-benchmarks-$${TAG}\n"]
  ephemeral_volumes = [{
    mount_path = "/scratch-data"
    size_gb    = 1000
    type       = "local-ssd"
  }]
  has_gpu                  = flatten([module.a4-pool.has_gpu])
  image                    = "ubuntu:22.04"
  k8s_service_account_name = "workload-identity-k8s-sa"
  labels                   = var.labels
  node_pool_names          = flatten([module.a4-pool.node_pool_names])
  persistent_volume_claims = flatten([module.training-pv.persistent_volume_claims, flatten([module.checkpointing-pv.persistent_volume_claims])])
  security_context = [{
    key   = "runAsUser"
    value = 0
    }, {
    key   = "runAsGroup"
    value = 100
    }, {
    key   = "fsGroup"
    value = 100
  }]
  tolerations          = flatten([module.a4-pool.tolerations])
  tpu_accelerator_type = flatten([module.a4-pool.tpu_accelerator_type])
  tpu_chips_per_node   = flatten([module.a4-pool.tpu_chips_per_node])
  tpu_topology         = flatten([module.a4-pool.tpu_topology])
}
