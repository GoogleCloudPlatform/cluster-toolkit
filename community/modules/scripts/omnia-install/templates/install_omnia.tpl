# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

---

- name: Creates SSH Keys to communicate between hosts
  hosts: localhost
  vars:
    username: ${username}
    pub_key_path: "/home/{{ username }}/.ssh"
    pub_key_file: "{{ pub_key_path }}/id_rsa"
    auth_key_file: "{{ pub_key_path }}/authorized_keys"
  tasks:
  - name: "Create {{ pub_key_path }} folder"
    ansible.builtin.file:
      path: "{{ pub_key_path }}"
      state: directory
      mode: 0700
      owner: "{{ username }}"
  - name: Create keys
    community.crypto.openssh_keypair:
      path: "{{ pub_key_file }}"
      owner: "{{ username }}"
  - name: Copy public key to authorized keys
    ansible.builtin.copy:
      src: "{{ pub_key_file }}.pub"
      dest: "{{ auth_key_file }}"
      owner: "{{ username }}"
      mode: 0644

- name: Install necessary dependencies
  hosts: localhost
  tasks:
  - name: Install git
    ansible.builtin.package:
      name:
      - git
      state: latest

- name: Prepare the system for Omnia installation
  hosts: localhost
  vars:
    install_dir: ${install_dir}
    omnia_dir: "{{ install_dir }}/omnia"
    slurm_uid: ${slurm_uid}
  tasks:
  - name: Git checkout
    ansible.builtin.git:
      repo: 'https://github.com/dellhpc/omnia.git'
      dest: "{{ omnia_dir }}"
      version: v1.3
      update: false
  - name: Copy inventory file with owner and permissions
    ansible.builtin.copy:
      src: "{{ install_dir }}/inventory"
      dest: "{{ omnia_dir }}/inventory"
      mode: 0644
  - name: Force update the ansible.utils collection
    command: ansible-galaxy collection install ansible.utils --force
  - name: Update omnia config to not use a login node
    ansible.builtin.lineinfile:
      path: "{{ omnia_dir }}/omnia_config.yml"
      regexp: '^login_node_required: .*'
      line: 'login_node_required: false'
  - name: Update omnia config to set the slurm UID
    ansible.builtin.lineinfile:
      path: "{{ omnia_dir }}/roles/slurm_common/vars/main.yml"
      regexp: '^slurm_uid: ".*"'
      line: 'slurm_uid: "{{ slurm_uid }}"'

- name: Run the Omnia installation once all nodes are ready
  hosts: localhost
  vars:
    nodecount: ${nodecount}
    install_dir: ${install_dir}
    username: ${username}
    venv: ${virtualenv_path}
    omnia_dir: "{{ install_dir }}/omnia"
    state_dir: "{{ install_dir }}/state"
  become_user: "{{ username }}"
  remote_user: "{{ username }}"
  tasks:
  - name: Wait for nodes to setup
    ansible.builtin.shell: |
      files=$(ls {{ state_dir }} | wc -l)
      if [ $files -eq ${nodecount} ]; then exit 0; fi
      echo "Waiting for ${nodecount} nodes to be ready, found $${files} nodes ready"
      exit 1
    delay: 2
    retries: 300
  - name: Checking if the provided virtualenv exists
    stat:
      path: "{{ venv }}"
    register: venv_dir
  - name: Run omnia using provided virtualenv for the python provider
    ansible.builtin.shell: |
      ansible-playbook omnia.yml \
        --private-key /home/{{ username }}/.ssh/id_rsa \
        --inventory inventory \
        --user "{{ username }}" --become \
        --e "ansible_python_interpreter={{ venv }}/bin/python3" \
        --skip-tags "kubernetes,nfs_client"
    args:
      chdir: "{{ omnia_dir }}"
    environment:
      ANSIBLE_HOST_KEY_CHECKING: False
    when: venv_dir.stat.exists
  - name: Run omnia
    ansible.builtin.shell: |
      ansible-playbook omnia.yml \
        --private-key /home/{{ username }}/.ssh/id_rsa \
        --inventory inventory \
        --user "{{ username }}" --become \
        --skip-tags "kubernetes,nfs_client"
    args:
      chdir: "{{ omnia_dir }}"
    environment:
      ANSIBLE_HOST_KEY_CHECKING: False
    when: not venv_dir.stat.exists
