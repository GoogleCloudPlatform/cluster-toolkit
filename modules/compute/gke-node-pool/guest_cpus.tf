/**
 * Copyright 2026 Google LLC
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

data "google_compute_machine_types" "machine_info" {
  for_each = var.zones == null ? toset([]) : toset(var.zones)

  project = var.project_id
  zone    = each.key
  filter  = "name = \"${var.machine_type}\""
}

locals {
  valid_machine_info = {
    for zone, data in data.google_compute_machine_types.machine_info :
    zone => data.machine_types if length(data.machine_types) > 0
  }

  guest_cpus = try(local.valid_machine_info[0].guest_cpus, 0)
}
