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
    prefix = "a4xhigh-slurm/sarthakaga4x/image-env"
  }
}

module "a4x-slurm-image-net" {
  source          = "./modules/embedded/modules/network/vpc"
  deployment_name = var.deployment_name
  labels          = var.labels
  project_id      = var.project_id
  region          = var.region
}

module "slurm-build-script" {
  source                         = "./modules/embedded/modules/scripts/startup-script"
  deployment_name                = var.deployment_name
  enable_gpu_network_wait_online = true
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
    content     = "ENROOT_CONFIG_PATH     $${HOME}/.enroot\n"
    destination = "/etc/enroot/enroot.conf"
    type        = "data"
    }, {
    content     = "* - memlock unlimited\n* - nproc unlimited\n* - stack 8192\n* - nofile 1048576\n* - cpu unlimited\n* - rtprio unlimited\n"
    destination = "/etc/security/limits.d/99-unlimited.conf"
    type        = "data"
    }, {
    content     = "---\n- name: Update OS settings prior to Slurm install\n  hosts: all\n  become: true\n  tasks:\n  - name: Turn off username space restriction in Apparmor\n    ansible.builtin.lineinfile:\n      path: /etc/sysctl.d/20-apparmor-donotrestrict.conf\n      regexp: '^kernel.apparmor_restrict_unprivileged_userns'\n      line: kernel.apparmor_restrict_unprivileged_userns = 0\n      create: yes\n    when: ansible_distribution == \"Ubuntu\" and  ansible_distribution_major_version is version('23', '>=')\n"
    destination = "update_settings.yml"
    type        = "ansible-local"
    }, {
    content     = "{\n  \"reboot\": false,\n  \"install_ompi\": true,\n  \"install_lustre\": false,\n  \"install_gcsfuse\": true,\n  \"install_cuda\": false,\n  \"allow_kernel_upgrades\": false,\n  \"monitoring_agent\": \"cloud-ops\",\n  install_managed_lustre: false,\n}\n"
    destination = "/var/tmp/slurm_vars.json"
    type        = "data"
    }, {
    content     = "#!/bin/bash\nset -e -o pipefail\nansible-pull \\\n    -U https://github.com/GoogleCloudPlatform/slurm-gcp -C ${var.build_slurm_from_git_ref} \\\n    -i localhost, --limit localhost --connection=local \\\n    -e @/var/tmp/slurm_vars.json \\\n    ansible/playbook.yml\n"
    destination = "install_slurm.sh"
    type        = "shell"
    }, {
    content     = "---\n- name: Install A4X Drivers and Utils\n  hosts: all\n  become: true\n  vars:\n    distribution: \"{{ ansible_distribution | lower }}{{ ansible_distribution_version | replace('.','') }}\"\n    cuda_repo_url: https://developer.download.nvidia.com/compute/cuda/repos/{{ distribution }}/sbsa/cuda-keyring_1.1-1_all.deb\n    cuda_repo_filename: /tmp/{{ cuda_repo_url | basename }}\n    nvidia_packages:\n    - cuda-toolkit-12-8\n    - datacenter-gpu-manager-4-cuda12\n    - datacenter-gpu-manager-4-dev\n  tasks:\n  - name: Download NVIDIA repository package\n    ansible.builtin.get_url:\n      url: \"{{ cuda_repo_url }}\"\n      dest: \"{{ cuda_repo_filename }}\"\n  - name: Install NVIDIA repository package\n    ansible.builtin.apt:\n      deb: \"{{ cuda_repo_filename }}\"\n      state: present\n  - name: Install NVIDIA fabric and CUDA\n    ansible.builtin.apt:\n      name: \"{{ item }}\"\n      update_cache: true\n      allow_downgrade: yes\n    loop: \"{{ nvidia_packages }}\"\n  - name: Freeze NVIDIA fabric and CUDA\n    ansible.builtin.command:\n      argv:\n      - apt-mark\n      - hold\n      - \"{{ item }}\"\n    loop: \"{{ nvidia_packages }}\"\n  - name: Create nvidia-persistenced override directory\n    ansible.builtin.file:\n      path: /etc/systemd/system/nvidia-persistenced.service.d\n      state: directory\n      owner: root\n      group: root\n      mode: 0o755\n  - name: Configure nvidia-persistenced override\n    ansible.builtin.copy:\n      dest: /etc/systemd/system/nvidia-persistenced.service.d/persistence_mode.conf\n      owner: root\n      group: root\n      mode: 0o644\n      content: |\n        [Service]\n        ExecStart=\n        ExecStart=/usr/bin/nvidia-persistenced --user nvidia-persistenced --verbose\n    notify: Reload SystemD\n  handlers:\n  - name: Reload SystemD\n    ansible.builtin.systemd:\n      daemon_reload: true\n  post_tasks:\n  - name: Disable NVIDIA DCGM by default (enable during boot on GPU nodes)\n    ansible.builtin.service:\n      name: nvidia-dcgm.service\n      state: stopped\n      enabled: false\n  - name: Disable nvidia-persistenced SystemD unit (enable during boot on GPU nodes)\n    ansible.builtin.service:\n      name: nvidia-persistenced.service\n      state: stopped\n      enabled: false\n"
    destination = "install_a4x_drivers.yml"
    type        = "ansible-local"
    }, {
    content     = "#!/bin/bash\nBASEMETADATAURL=http://metadata.google.internal/computeMetadata/v1/instance/\nrm $(curl -f -H \"Metadata-Flavor: Google\" $${BASEMETADATAURL}/attributes/startup-script-log-dest 2> /dev/null)\ngcloud compute instances add-metadata $(hostname -s) --metadata \"startup-script-status\"=\"done\" --zone ${var.zone}\n"
    destination = "stop_packer_early.sh"
    type        = "shell"
  }]
}
