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

- name: Install dependencies for ramble installation
  become: yes
  hosts: localhost
  vars:
    ramble_ref: ${ramble_ref}
  tasks:
  - name: Install dependencies through system package manager
    ansible.builtin.package:
      name:
      - python
      - python3-pip
      - git

  - name: Gather the package facts
    ansible.builtin.package_facts:
      manager: auto

  - name: Install protobuf for old releases of Python
    when: ansible_facts.packages["python3"][0].version is version("3.7", "<") and ansible_facts.packages["python3"][0].version is version("3.5", ">=")
    ansible.builtin.pip:
      name: protobuf
      version: 3.19.4
      executable: pip3

  - name: Download ramble requirements file
    ansible.builtin.get_url:
      url: "https://raw.githubusercontent.com/GoogleCloudPlatform/ramble/{{ ramble_ref }}/requirements.txt"
      dest: /tmp/requirements.txt

  - name: Install ramble dependencies
    ansible.builtin.pip:
      requirements: /tmp/requirements.txt
