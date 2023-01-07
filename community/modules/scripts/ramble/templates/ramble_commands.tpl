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

---

- name: Run Ramble Commands
  hosts: localhost
  vars:
    spack_path: ${spack_path}
    command_prefix: ${command_prefix}
%{if length(COMMANDS) > 0 ~}
    commands:
%{for c in COMMANDS ~}
    - ${c}
%{endfor ~}
%{else ~}
    commands: []
%{endif ~}

    install_dir: ${install_dir}
    log_file: ${log_file}
  tasks:
  - name: Run ramble commands # Might be able to remove the spack setup after spack is installed properly.
    ansible.builtin.shell: |
      . {{ spack_path }}/share/spack/setup-env.sh
      . {{ install_dir }}/share/ramble/setup-env.sh
      echo "" >> {{ log_file }}
      echo " === Running command: {{ command_prefix }} {{ item }} === " >> {{ log_file }}
      {{ command_prefix }} {{ item }} >> {{ log_file }}

    loop: "{{ commands }}"
