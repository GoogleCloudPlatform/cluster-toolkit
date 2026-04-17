# Copyright 2025 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "commands" {
  description = "A list of gcloud command pairs for creation and destruction."
  type = list(object({
    create = string
    delete = string
  }))
  default = []
}

variable "module_instance_id" {

  description = "The unique ID of this module instance in the blueprint. This is automatically populated by gcluster."

  type = string

}
