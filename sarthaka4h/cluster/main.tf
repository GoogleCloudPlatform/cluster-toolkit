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
    prefix = "a4high-slurm/sarthaka4h/cluster"
  }
}

module "a4high_startup" {
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
  runners = [var.client_install_runner_gcs_checkpoints, var.mount_runner_gcs_checkpoints, var.client_install_runner_gcs_training_data, var.mount_runner_gcs_training_data, var.client_install_runner_gcs_model_serving, var.mount_runner_gcs_model_serving, {
    content     = "ENROOT_CONFIG_PATH     $${HOME}/.enroot\nENROOT_RUNTIME_PATH    ${var.local_ssd_mountpoint}/$${UID}/enroot/runtime\nENROOT_CACHE_PATH      ${var.local_ssd_mountpoint}/$${UID}/enroot/cache\nENROOT_DATA_PATH       ${var.local_ssd_mountpoint}/$${UID}/enroot/data\nENROOT_TEMP_PATH       ${var.local_ssd_mountpoint}/$${UID}/enroot\n"
    destination = "/etc/enroot/enroot.conf"
    type        = "data"
    }, {
    content     = "---\n- name: Install nccl\n  hosts: all\n  become: true\n  tasks:\n  - name: Update apt cache\n    ansible.builtin.apt:\n      update_cache: yes\n  - name: Install Linux Modules Extra\n    ansible.builtin.package:\n      name:\n      - \"libnccl2=${var.libnccl_version}\"\n      - \"libnccl-dev=${var.libnccl_version}\"\n      state: present\n"
    destination = "install_nccl.yml"
    type        = "ansible-local"
    }, {
    content     = "---\n- name: Install Google NCCL-GIB Plugin\n  hosts: all\n  become: true\n  tasks:\n  - name: Add artifact registry gpg key\n    ansible.builtin.apt_key:\n      url: https://us-apt.pkg.dev/doc/repo-signing-key.gpg\n      state: present\n  - name: Add artifact registry gpg key\n    ansible.builtin.apt_key:\n      url: https://packages.cloud.google.com/apt/doc/apt-key.gpg\n      state: present\n  - name: Install Apt Transport AR Apt Repo\n    apt_repository:\n      repo: 'deb http://packages.cloud.google.com/apt apt-transport-artifact-registry-stable main'\n      state: present\n  - name: Install AR transport\n    ansible.builtin.apt:\n      name: \"apt-transport-artifact-registry\"\n      update_cache: true\n  - name: Install Google NCCL-GIB Plugin\n    apt_repository:\n      repo: \"deb ar+https://us-apt.pkg.dev/projects/gce-ai-infra gpudirect-gib-apt main\"\n      state: present\n  - name: Install NCCL-GIB Plugin\n    ansible.builtin.apt:\n      name: \"nccl-gib=${var.nccl_gib_version}\"\n      update_cache: true\n  - name: Freeze NCCL GIB Plugin\n    ansible.builtin.dpkg_selections:\n      name: \"nccl-gib\"\n      selection: hold\n"
    destination = "install_nccl_gib.yml"
    type        = "ansible-local"
    }, {
    content     = "---\n- name: Configure NCCL/gIB environment\n  hosts: all\n  become: true\n  tasks:\n    - name: Deploy /etc/profile.d/nccl-gib.sh\n      ansible.builtin.copy:\n        dest: /etc/profile.d/nccl-gib.sh\n        content: |\n          # Load NCCL/gIB environment\n          if [ -f \"/usr/local/gib/scripts/set_nccl_env.sh\" ]; then\n            source /usr/local/gib/scripts/set_nccl_env.sh\n          fi\n\n          # Ensure /usr/local/gib/lib64 is in LD_LIBRARY_PATH\n          if [ -d \"/usr/local/gib/lib64\" ]; then\n            export LD_LIBRARY_PATH=\"/usr/local/gib/lib64$${LD_LIBRARY_PATH:+:$${LD_LIBRARY_PATH}}\"\n          fi\n        mode: '0644'\n  handlers:\n    - name: Reload SystemD\n      ansible.builtin.systemd:\n        daemon_reload: true\n"
    destination = "configure_nccl_env.yml"
    name        = "Ensure NCCL/gIB environment script is sourced for all users"
    type        = "ansible-local"
    }, {
    content     = "---\n- name: Enable NVIDIA DCGM on GPU nodes\n  hosts: all\n  become: true\n  vars:\n    enable_ops_agent: true\n    enable_nvidia_dcgm: true\n    enable_nvidia_persistenced: true\n  tasks:\n  - name: Update Ops Agent configuration\n    ansible.builtin.blockinfile:\n      path: /etc/google-cloud-ops-agent/config.yaml\n      insertafter: EOF\n      block: |\n        metrics:\n          receivers:\n            dcgm:\n              type: dcgm\n          service:\n            pipelines:\n              dcgm:\n                receivers:\n                  - dcgm\n    notify:\n    - Restart Google Cloud Ops Agent\n  handlers:\n  - name: Restart Google Cloud Ops Agent\n    ansible.builtin.service:\n      name: google-cloud-ops-agent.service\n      state: \"{{ 'restarted' if enable_ops_agent else 'stopped' }}\"\n      enabled: \"{{ enable_ops_agent }}\"\n  post_tasks:\n  - name: Enable Google Cloud Ops Agent\n    ansible.builtin.service:\n      name: google-cloud-ops-agent.service\n      state: \"{{ 'started' if enable_ops_agent else 'stopped' }}\"\n      enabled: \"{{ enable_ops_agent }}\"\n  - name: Enable NVIDIA DCGM\n    ansible.builtin.service:\n      name: nvidia-dcgm.service\n      state: \"{{ 'started' if enable_nvidia_dcgm else 'stopped' }}\"\n      enabled: \"{{ enable_nvidia_dcgm }}\"\n  - name: Enable NVIDIA Persistence Daemon\n    ansible.builtin.service:\n      name: nvidia-persistenced.service\n      state: \"{{ 'started' if enable_nvidia_persistenced else 'stopped' }}\"\n      enabled: \"{{ enable_nvidia_persistenced }}\"\n"
    destination = "enable_dcgm.yml"
    type        = "ansible-local"
  }]
}

module "a4high_nodeset" {
  source              = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-nodeset"
  additional_networks = concat([{ network = null, subnetwork = var.subnetwork_self_link_a4high-slurm-net-1, subnetwork_project = var.project_id, nic_type = "GVNIC", queue_count = null, network_ip = "", stack_type = null, access_config = [], ipv6_access_config = [], alias_ip_range = [] }], var.subnetwork_interfaces_a4high-slurm-rdma-net)
  advanced_machine_features = {
    threads_per_core = null
  }
  bandwidth_tier = "gvnic_enabled"
  disk_size_gb   = var.disk_size_gb
  disk_type      = "hyperdisk-balanced"
  dws_flex = {
    enabled = var.a4h_dws_flex_enabled
  }
  enable_placement  = false
  enable_public_ips = true
  enable_spot_vm    = var.a4h_enable_spot_vm
  instance_image    = var.instance_image
  labels            = var.labels
  machine_type      = "a4-highgpu-8g"
  name              = "a4high_nodeset"
  node_conf = {
    CoresPerSocket  = 56
    SocketsPerBoard = 2
    ThreadsPerCore  = 2
  }
  node_count_dynamic_max = 0
  node_count_static      = var.a4h_cluster_size
  on_host_maintenance    = "TERMINATE"
  project_id             = var.project_id
  region                 = var.region
  reservation_name       = var.a4h_reservation_name
  startup_script         = module.a4high_startup.startup_script
  subnetwork_self_link   = var.subnetwork_self_link_a4high-slurm-net-0
  zone                   = var.zone
}

module "a4high_partition" {
  source     = "./modules/embedded/community/modules/compute/schedmd-slurm-gcp-v6-partition"
  exclusive  = false
  is_default = true
  nodeset    = flatten([module.a4high_nodeset.nodeset])
  partition_conf = {
    OverSubscribe  = "EXCLUSIVE"
    ResumeTimeout  = 1200
    SuspendTimeout = 1200
  }
  partition_name = "a4high"
}

module "slurm_login" {
  source                  = "./modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-login"
  disk_size_gb            = 300
  enable_login_public_ips = true
  instance_image          = var.instance_image
  labels                  = var.labels
  machine_type            = "n2-standard-8"
  name_prefix             = "slurm_login"
  project_id              = var.project_id
  region                  = var.region
  subnetwork_self_link    = var.subnetwork_self_link_a4high-slurm-net-0
  zone                    = var.zone
}

module "controller_startup" {
  source          = "./modules/embedded/modules/scripts/startup-script"
  deployment_name = var.deployment_name
  labels          = var.labels
  project_id      = var.project_id
  region          = var.region
  runners = [{
    content     = "#!/bin/bash\nSLURM_ROOT=/opt/apps/adm/slurm\nPARTITION_NAME=${module.a4high_partition.partitions[0].partition_name}\nmkdir -m 0755 -p \"$${SLURM_ROOT}/scripts\"\n# enable a GPU health check that runs at the completion of all jobs on A4 nodes\nmkdir -p \"$${SLURM_ROOT}/partition-$${PARTITION_NAME}-epilog_slurmd.d\"\nln -s \"/slurm/scripts/tools/gpu-test\" \"$${SLURM_ROOT}/partition-$${PARTITION_NAME}-epilog_slurmd.d/gpu-test.epilog_slurmd\"\n# enable the use of password-free sudo within Slurm jobs on all compute nodes\n# feature is restricted to users with OS Admin Login IAM role\n# https://cloud.google.com/iam/docs/understanding-roles#compute.osAdminLogin\nmkdir -p \"$${SLURM_ROOT}/prolog_slurmd.d\"\nmkdir -p \"$${SLURM_ROOT}/epilog_slurmd.d\"\ncurl -s -o \"$${SLURM_ROOT}/scripts/sudo-oslogin\" \\\n    https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/master/tools/prologs-epilogs/sudo-oslogin\nchmod 0755 \"$${SLURM_ROOT}/scripts/sudo-oslogin\"\nln -s \"$${SLURM_ROOT}/scripts/sudo-oslogin\" \"$${SLURM_ROOT}/prolog_slurmd.d/sudo-oslogin.prolog_slurmd\"\nln -s \"$${SLURM_ROOT}/scripts/sudo-oslogin\" \"$${SLURM_ROOT}/epilog_slurmd.d/sudo-oslogin.epilog_slurmd\"\n"
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
  source                        = "./modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-controller"
  controller_startup_script     = module.controller_startup.startup_script
  deployment_name               = var.deployment_name
  disk_size_gb                  = 300
  disk_type                     = "pd-extreme"
  enable_controller_public_ips  = true
  enable_external_prolog_epilog = true
  instance_image                = var.instance_image
  labels                        = var.labels
  login_nodes                   = flatten([module.slurm_login.login_nodes])
  machine_type                  = "n2-standard-80"
  network_storage               = flatten([var.network_storage_gcs_bucket, flatten([var.network_storage_homefs])])
  nodeset                       = flatten([module.a4high_partition.nodeset])
  nodeset_dyn                   = flatten([module.a4high_partition.nodeset_dyn])
  nodeset_tpu                   = flatten([module.a4high_partition.nodeset_tpu])
  partitions                    = flatten([module.a4high_partition.partitions])
  project_id                    = var.project_id
  region                        = var.region
  subnetwork_self_link          = var.subnetwork_self_link_a4high-slurm-net-0
  zone                          = var.zone
}
