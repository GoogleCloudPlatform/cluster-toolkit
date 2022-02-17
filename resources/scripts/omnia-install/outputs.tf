/**
 * Copyright 2021 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

output "inventory_file" {
  description = "The inventory file for the omnia cluster"
  value       = local.inventory
}

output "add_omnia_user_script" {
  description = "An ansible script that adds the user that install omnia"
  value       = local.add_user_file
}

output "runners" {
  description = "The runners to setup and install omnia on the manager"
  value       = local.startup_runners
}
