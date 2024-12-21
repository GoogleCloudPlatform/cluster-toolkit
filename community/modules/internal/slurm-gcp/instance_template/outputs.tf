# Copyright 2024 Google LLC
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

output "instance_template" {
  description = "Instance template details"
  value       = module.instance_template
}

output "self_link" {
  description = "Self_link of instance template"
  value       = module.instance_template.self_link
}

output "name" {
  description = "Name of instance template"
  value       = module.instance_template.name
}

output "tags" {
  description = "Tags that will be associated with instance(s)"
  value       = module.instance_template.tags
}

output "service_account" {
  description = "Service account object, includes email and scopes."
  value       = module.instance_template.service_account
}
