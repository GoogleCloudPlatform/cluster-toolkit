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
- name: Create user for installing Omnia
  hosts: localhost
  vars:
    username: ${username}
  tasks:
  - name: Create a new user
    ansible.builtin.user:
      name: "{{ username }}"
  - name: Allow '{{ username }}' user to have passwordless sudo access
    ansible.builtin.lineinfile:
      dest: /etc/sudoers
      state: present
      regexp: '^%%{{ username }}'
      line: '%%{{ username }} ALL=(ALL) NOPASSWD: ALL'

- name: Setup selinux
  hosts: localhost
  tasks:
  - name: Install selinux using system pip
    ansible.builtin.pip:
      name: selinux
  - name: Allow SSH on NFS-based home directory
    ansible.builtin.command: setsebool -P use_nfs_home_dirs 1

- name: Set Status file
  hosts: localhost
  vars:
    install_dir: ${install_dir}
    state_dir: "{{ install_dir }}/state"
  tasks:
  - name: Get hostname
    ansible.builtin.command: hostname
    register: machine_hostname
  - name: Create state dir if not already created
    ansible.builtin.file:
      path: "{{ state_dir }}"
      state: directory
  - name: Create file
    ansible.builtin.file:
      path: "{{ state_dir }}/{{ machine_hostname.stdout }}"
      state: touch
