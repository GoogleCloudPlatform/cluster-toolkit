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

- name: Execute Commands
  hosts: localhost
  vars:
    pre_script: ${pre_script}
    log_file: ${log_file}
    commands: ${commands}
    system_user_name: ${system_user_name}
  tasks:
  - name: Execute command block
    block:
    - name: Print commands to be executed
      ansible.builtin.debug:
        msg: "{{ commands.split('\n') | ansible.builtin.to_nice_yaml }}"

    - name: Streaming log info
      ansible.builtin.debug:
        msg: |
          Logs from commands will not be printed here until success (or failure)
          Streaming logs can be found at {{ log_file }}

    - name: Ensure user can write to log file
      ansible.builtin.file:
        path: "{{ log_file }}"
        state: touch
        owner: "{{ system_user_name }}"

    - name: Execute commands
      ansible.builtin.shell: |
        set -eo pipefail
        {
        {{ pre_script }}
        echo " === Starting commands ==="
        {{ commands }}
        echo " === Finished commands ==="
        } 2>&1 | tee -a {{ log_file }}
      args:
        executable: /bin/bash
      register: output
      become: true
      become_user: "{{ system_user_name }}"

    always:
    - name: Print commands output
      ansible.builtin.debug:
        var: output.stdout_lines
