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
    prefix = "a4high-slurm/sarthaka4h/image-env"
  }
}

module "slurm-image-network" {
  source          = "./modules/embedded/modules/network/vpc"
  deployment_name = var.deployment_name
  labels          = var.labels
  network_name    = "${var.base_network_name}-net"
  project_id      = var.project_id
  region          = var.region
}

module "slurm-build-script" {
  source          = "./modules/embedded/modules/scripts/startup-script"
  deployment_name = var.deployment_name
  docker = {
    enabled        = true
    world_writable = true
  }
  enable_gpu_network_wait_online = true
  install_ansible                = true
  labels                         = var.labels
  project_id                     = var.project_id
  region                         = var.region
  runners = [{
    content     = "Package: nvidia-container-toolkit nvidia-container-toolkit-base libnvidia-container-tools libnvidia-container1\nPin: version 1.17.7-1\nPin-Priority: 100\n"
    destination = "/etc/apt/preferences.d/block-broken-nvidia-container"
    type        = "data"
    }, {
    content     = "---\n- name: Hold nvidia packages\n  hosts: all\n  become: true\n  vars:\n    nvidia_packages_to_hold:\n    - libnvidia-cfg1-*-server\n    - libnvidia-compute-*-server\n    - libnvidia-nscq-*\n    - nvidia-compute-utils-*-server\n    - nvidia-fabricmanager-*\n    - nvidia-utils-*-server\n    - nvidia-imex-*\n  tasks:\n  - name: Hold nvidia packages\n    ansible.builtin.command:\n      argv:\n      - apt-mark\n      - hold\n      - \"{{ item }}\"\n    loop: \"{{ nvidia_packages_to_hold }}\"\n"
    destination = "hold-nvidia-packages.yml"
    type        = "ansible-local"
    }, {
    content     = "{\n  \"reboot\": false,\n  \"install_cuda\": false,\n  \"install_gcsfuse\": true,\n  \"install_lustre\": false,\n  \"install_managed_lustre\": false,\n  \"install_nvidia_repo\": true,\n  \"install_ompi\": true,\n  \"allow_kernel_upgrades\": false,\n  \"monitoring_agent\": \"cloud-ops\",\n}\n"
    destination = "/var/tmp/slurm_vars.json"
    type        = "data"
    }, {
    content     = "#!/bin/bash\nset -e -o pipefail\nansible-pull \\\n    -U https://github.com/GoogleCloudPlatform/slurm-gcp -C ${var.build_slurm_from_git_ref} \\\n    -i localhost, --limit localhost --connection=local \\\n    -e @/var/tmp/slurm_vars.json \\\n    ansible/playbook.yml\n"
    destination = "install_slurm.sh"
    type        = "shell"
    }, {
    content     = "* - memlock unlimited\n* - nproc unlimited\n* - stack unlimited\n* - nofile 1048576\n* - cpu unlimited\n* - rtprio unlimited\n"
    destination = "/etc/security/limits.d/99-unlimited.conf"
    type        = "data"
    }, {
    content     = "#!/bin/bash\nset -ex -o pipefail\nadd-nvidia-repositories -y\napt update -y\napt install -y cuda-toolkit-12-8\napt install -y datacenter-gpu-manager-4-cuda12\napt install -y datacenter-gpu-manager-4-dev\n"
    destination = "install-cuda-toolkit.sh"
    type        = "shell"
    }, {
    content     = "---\n- name: CUDA and DGMA settings\n  hosts: all\n  become: true\n  vars:\n    enable_nvidia_dcgm: false\n  tasks:\n  - name: Reduce NVIDIA repository priority\n    ansible.builtin.copy:\n      dest: /etc/apt/preferences.d/cuda-repository-pin-600\n      mode: 0o0644\n      owner: root\n      group: root\n      content: |\n        Package: *\n        Pin: release l=NVIDIA CUDA\n        Pin-Priority: 400\n  - name: Create nvidia-persistenced override directory\n    ansible.builtin.file:\n      path: /etc/systemd/system/nvidia-persistenced.service.d\n      state: directory\n      owner: root\n      group: root\n      mode: 0o755\n  - name: Configure nvidia-persistenced override\n    ansible.builtin.copy:\n      dest: /etc/systemd/system/nvidia-persistenced.service.d/persistence_mode.conf\n      owner: root\n      group: root\n      mode: 0o644\n      content: |\n        [Service]\n        ExecStart=\n        ExecStart=/usr/bin/nvidia-persistenced --user nvidia-persistenced --verbose\n    notify: Reload SystemD\n  handlers:\n  - name: Reload SystemD\n    ansible.builtin.systemd:\n      daemon_reload: true\n  post_tasks:\n  - name: Disable NVIDIA DCGM by default (enable during boot on GPU nodes)\n    ansible.builtin.service:\n      name: nvidia-dcgm.service\n      state: stopped\n      enabled: \"{{ enable_nvidia_dcgm }}\"\n  - name: Disable nvidia-persistenced SystemD unit (enable during boot on GPU nodes)\n    ansible.builtin.service:\n      name: nvidia-persistenced.service\n      state: stopped\n      enabled: false\n"
    destination = "configure_cuda_dcgm.yml"
    type        = "ansible-local"
    }, {
    content     = "---\n- name: Install ibverbs-utils\n  hosts: all\n  become: true\n  tasks:\n  - name: Install Linux Modules Extra\n    ansible.builtin.package:\n      name:\n      - ibverbs-utils\n      state: present\n"
    destination = "install_ibverbs_utils.yml"
    type        = "ansible-local"
    }, {
    content     = "ENROOT_CONFIG_PATH     $${HOME}/.enroot\n"
    destination = "/etc/enroot/enroot.conf"
    type        = "data"
  }]
}
