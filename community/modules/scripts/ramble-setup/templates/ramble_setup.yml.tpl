# Copyright 2023 Google LLC
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

- name: Install Ramble
  hosts: localhost
  vars:
    install_dir: ${install_dir}
    ramble_url: ${ramble_url}
    ramble_ref: ${ramble_ref}
    chmod_mode: ${chmod_mode}
    chown_owner: ${chown_owner}
    chgrp_group: ${chgrp_group}
  tasks:
  - name: Create parent of install directory
    ansible.builtin.file:
      path: "{{ install_dir | dirname }}"
      state: directory

  - name: Acquire lock
    ansible.builtin.command:
      mkdir "{{ install_dir | dirname }}/.ramble_lock"
    register: lock_out
    changed_when: lock_out.rc == 0
    failed_when: false

  - name: Clones ramble into installation directory
    ansible.builtin.git:
      repo: "{{ ramble_url }}"
      dest: "{{ install_dir }}"
      version: "{{ ramble_ref }}"
    when: lock_out.rc == 0

  - name: chgrp ramble installation
    ansible.builtin.file:
      path: "{{ install_dir }}"
      group: "{{ chgrp_group }}"
      recurse: true
    when: chgrp_group != "" and lock_out.rc == 0

  - name: chown ramble installation
    ansible.builtin.file:
      path: "{{ install_dir }}"
      owner: "{{ chown_owner }}"
      recurse: true
    when: chown_owner != "" and lock_out.rc == 0

  - name: chmod ramble installation
    ansible.builtin.file:
      path: "{{ install_dir }}"
      mode: "{{ chmod_mode }}"
      recurse: true
    when: chmod_mode != "" and lock_out.rc == 0

  - name: Check if ramble profile exists
    ansible.builtin.stat:
      path: /etc/profile.d/ramble.sh
    register: profile_check

  - name: Add ramble to profile
    ansible.builtin.copy:
      dest: /etc/profile.d/ramble.sh
      content: ". {{ install_dir }}/share/ramble/setup-env.sh"
    when: not profile_check.stat.exists
