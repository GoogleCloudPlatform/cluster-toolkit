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

- name: Execute Ramble Commands
  hosts: localhost
  vars:
    spack_path: ${spack_path}
    ramble_path: ${ramble_path}
    log_file: ${log_file}
  tasks:
  - name: Execute ramble commands
    ansible.builtin.shell: |
      . {{ spack_path }}/share/spack/setup-env.sh
      . {{ ramble_path }}/share/ramble/setup-env.sh
      echo "" >> {{ log_file }}
      echo " === Starting ramble commands ===" >> {{ log_file }}
%{if length(COMMANDS) > 0 ~}
%{for c in COMMANDS ~}
      echo " === Executing command: ${c} === " >> {{ log_file }}
      ${c} >> {{ log_file }}
%{endfor ~}
%{endif ~}
      echo " === Finished ramble commands ===" >> {{ log_file }}
