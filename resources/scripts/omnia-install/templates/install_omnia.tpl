# Copyright 2021 Google LLC
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
## TODO Make this into a templatefile - add variables via terraform. Same with add_omnia_user

- name: Creates SSH Keys to communicate between hosts
  hosts: localhost
  vars:
    pub_key_path: /home/omnia/.ssh
    pub_key_file: "{{pub_key_path}}/id_rsa"
    auth_key_file: "{{pub_key_path}}/authorized_keys"
    username: ${username}
  tasks:
  - name: "Create {{pub_key_path}} folder"
    file:
      path: "{{pub_key_path}}"
      state: directory
      mode: 0700
      owner: "{{username}}"
  - name: Create keys
    openssh_keypair:
      path: "{{pub_key_file}}"
      owner: "{{username}}"
  - name: Copy public key to authorized keys
    copy:
      src: "{{pub_key_file}}.pub"
      dest: "{{auth_key_file}}"
      owner: "{{username}}"
      mode: 0644

- name: install_deps
  hosts: localhost
  vars:
    username: ${username}
  become_user: "{{ username }}"
  tasks:
  - name: Install git and epel-release
    package:
      name:
      - git
      - epel-release
      state: latest

- name: setup_omnia
  hosts: localhost
  vars:
    install_dir: ${install_dir}
    repo_name: omnia
    omnia_dir: "{{ install_dir }}/omnia"
  tasks:
  - name: Unmask and restart firewalld
    become: true
    command: systemctl unmask firewalld && systemctl restart firewalld
  - name: Git checkout
    git:
      repo: 'https://github.com/dellhpc/omnia.git'
      dest: "{{ omnia_dir }}/{{ repo_name }}"
      version: release-1.0
      update: false
  - name: Copy inventory file with owner and permissions
    copy:
      src: "{{ omnia_dir }}/inventory"
      dest: "{{ omnia_dir }}/{{ repo_name }}/inventory"
      mode: 0644
  - name: Update omnia.yml setting become to yes
    replace:
      path: "{{ omnia_dir }}/{{ repo_name }}/omnia.yml"
      regexp: '- name(.*)'
      replace: '- name\1\n  become: yes'

- name: run_omnia  # Look into import_playbook here
  hosts: localhost
  vars:
    install_dir: ${install_dir}
    username: ${username}
    omnia_dir: "{{ install_dir }}/omnia"
  become_user: "{{ username }}"
  remote_user: "{{ username }}"
  tasks:
  - name: Run omnia
    shell: |
      ansible-playbook omnia.yml \
        --private-key /home/{{ username }}/.ssh/id_rsa \
        --inventory inventory \
        --user "{{ username }}" \
        --e "ansible_python_interpreter=/usr/bin/python2" \
        --skip-tags "kubernetes"
    args:
      chdir: "{{ omnia_dir }}/omnia"
    environment:
      ANSIBLE_HOST_KEY_CHECKING: False
