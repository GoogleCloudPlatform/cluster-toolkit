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

- name: Setup ramble access
  hosts: localhost
  vars:
    ramble_path: ${ramble_path}
  tasks:
  - name: create ramble profile file
    ansible.builtin.shell:
      cmd: echo ". {{ ramble_path }}/share/ramble/setup-env.sh" > /etc/profile.d/ramble.sh
