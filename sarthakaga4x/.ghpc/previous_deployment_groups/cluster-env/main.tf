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
    prefix = "a4xhigh-slurm/sarthakaga4x/cluster-env"
  }
}

module "a4x-slurm-net-0" {
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
    name   = "${var.deployment_name}-internal-0"
    ranges = [var.net0_range]
  }]
  labels       = var.labels
  mtu          = 8896
  network_name = "${var.deployment_name}-net-0"
  project_id   = var.project_id
  region       = var.region
  subnetworks = [{
    subnet_ip     = var.net0_range
    subnet_name   = "${var.deployment_name}-sub-0"
    subnet_region = var.region
  }]
}

module "a4x-slurm-net-1" {
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
    name   = "${var.deployment_name}-internal-1"
    ranges = [var.net1_range]
  }]
  labels       = var.labels
  mtu          = 8896
  network_name = "${var.deployment_name}-net-1"
  project_id   = var.project_id
  region       = var.region
  subnetworks = [{
    subnet_ip     = var.net1_range
    subnet_name   = "${var.deployment_name}-sub-1"
    subnet_region = var.region
  }]
}

module "a4x-slurm-rdma-net" {
  source               = "./modules/embedded/modules/network/gpu-rdma-vpc"
  deployment_name      = var.deployment_name
  network_name         = "${var.deployment_name}-rdma-net"
  network_profile      = "https://www.googleapis.com/compute/beta/projects/${var.project_id}/global/networkProfiles/${var.zone}-vpc-roce"
  network_routing_mode = "REGIONAL"
  project_id           = var.project_id
  region               = var.region
  subnetworks_template = {
    count       = 4
    ip_range    = var.rdma_net_range
    name_prefix = "${var.deployment_name}-mrdma-sub"
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
  network_id        = module.a4x-slurm-net-0.network_id
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

module "a4x_startup" {
  source          = "./modules/embedded/modules/scripts/startup-script"
  deployment_name = var.deployment_name
  docker = {
    daemon_config  = "{\n  \"data-root\": \"${var.local_ssd_mountpoint}/docker\"\n}\n"
    enabled        = true
    world_writable = true
  }
  labels = var.labels
  local_ssd_filesystem = {
    mountpoint  = var.local_ssd_mountpoint
    permissions = "1777"
  }
  project_id = var.project_id
  region     = var.region
  runners = [module.gcs_checkpoints.client_install_runner, module.gcs_checkpoints.mount_runner, module.gcs_training_data.client_install_runner, module.gcs_training_data.mount_runner, module.gcs_model_serving.client_install_runner, module.gcs_model_serving.mount_runner, {
    content     = "#!/bin/bash\nmkdir -p /mnt/localssd\nchmod 1777 /mnt/localssd\n"
    destination = "ensure_mnt_localssd_permissions.sh"
    type        = "shell"
    }, {
    content     = "#!/bin/bash\nhostname -s | sudo tee /etc/hostname > /dev/null\n"
    destination = "ensure_etc_hostname_created.sh"
    type        = "shell"
    }, {
    content     = "optional /usr/lib/aarch64-linux-gnu/slurm/spank_pyxis.so\n"
    destination = "/etc/slurm/plugstack.conf.d/pyxis.conf"
    type        = "data"
    }, {
    content     = "ENROOT_CONFIG_PATH     $${HOME}/.enroot\nENROOT_RUNTIME_PATH    ${var.local_ssd_mountpoint}/$${UID}/enroot/runtime\nENROOT_CACHE_PATH      ${var.local_ssd_mountpoint}/$${UID}/enroot/cache\nENROOT_DATA_PATH       ${var.local_ssd_mountpoint}/$${UID}/enroot/data\nENROOT_TEMP_PATH       ${var.local_ssd_mountpoint}/$${UID}/enroot\n"
    destination = "/etc/enroot/enroot.conf"
    type        = "data"
    }, {
    content     = "---\n- name: Install NCCL plugin for A4X series\n  hosts: all\n  become: true\n  tasks:\n  - name: Add SystemD unit for NCCL plugin installation\n    ansible.builtin.copy:\n      dest: /etc/systemd/system/nccl-plugin@${var.nccl_plugin_version}.service\n      mode: 0o0644\n      content: |\n        [Unit]\n        After=network-online.target docker.service\n        Before=slurmd.service\n        Requires=docker.service\n\n        [Service]\n        Type=oneshot\n        ExecStartPre=/usr/bin/rm -rf /usr/local/gib\n        ExecStartPre=/usr/bin/mkdir -p /usr/local/gib\n        ExecStartPre=/snap/bin/gcloud auth configure-docker --quiet us-docker.pkg.dev\n        ExecStart=/usr/bin/docker run --rm --name nccl-gib-installer --volume /usr/local/gib:/var/lib/gib \\\n            us-docker.pkg.dev/gce-ai-infra/gpudirect-gib/nccl-plugin-gib-arm64:latest install --install-nccl\n        ExecStartPost=/usr/bin/chmod -R a+r /usr/local/gib\n\n        [Install]\n        WantedBy=slurmd.service\n    notify:\n    - Reload SystemD\n  handlers:\n  - name: Reload SystemD\n    ansible.builtin.systemd:\n      daemon_reload: true\n  post_tasks:\n  - name: Enable NCCL plugin SystemD unit\n    ansible.builtin.service:\n      name: nccl-plugin@${var.nccl_plugin_version}.service\n      state: started\n      enabled: true\n"
    destination = "nccl_plugin.yml"
    type        = "ansible-local"
    }, {
    content     = "---\n- name: Enable NVIDIA DCGM on GPU nodes\n  hosts: all\n  become: true\n  vars:\n    enable_ops_agent: true\n    enable_nvidia_dcgm: true\n  tasks:\n  - name: Update Ops Agent configuration\n    ansible.builtin.blockinfile:\n      path: /etc/google-cloud-ops-agent/config.yaml\n      insertafter: EOF\n      block: |\n        metrics:\n          receivers:\n            dcgm:\n              type: dcgm\n          service:\n            pipelines:\n              dcgm:\n                receivers:\n                  - dcgm\n    notify:\n    - Restart Google Cloud Ops Agent\n  handlers:\n  - name: Restart Google Cloud Ops Agent\n    ansible.builtin.service:\n      name: google-cloud-ops-agent.service\n      state: \"{{ 'restarted' if enable_ops_agent else 'stopped' }}\"\n      enabled: \"{{ enable_ops_agent }}\"\n  post_tasks:\n  - name: Enable Google Cloud Ops Agent\n    ansible.builtin.service:\n      name: google-cloud-ops-agent.service\n      state: \"{{ 'started' if enable_ops_agent else 'stopped' }}\"\n      enabled: \"{{ enable_ops_agent }}\"\n  - name: Enable NVIDIA DCGM\n    ansible.builtin.service:\n      name: nvidia-dcgm.service\n      state: \"{{ 'started' if enable_nvidia_dcgm else 'stopped' }}\"\n      enabled: \"{{ enable_nvidia_dcgm }}\"\n  # Enable persistenced service\n  - name: Enable nvidia-persistenced\n    ansible.builtin.service:\n      name: nvidia-persistenced.service\n      state: started\n      enabled: true\n"
    destination = "enable_dcgm.yml"
    type        = "ansible-local"
  }]
}
