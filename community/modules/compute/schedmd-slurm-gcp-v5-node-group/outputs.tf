/**
 * Copyright 2022 Google LLC
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

output "node_groups" {
  description = "Details of the node group. Typically used as input to `schedmd-slurm-gcp-v5-partition`."
  value       = local.node_group

  precondition {
    condition = !contains([
      "c3-:pd-standard",
      "h3-:pd-standard",
      "h3-:pd-ssd",
    ], "${substr(var.machine_type, 0, 3)}:${var.disk_type}")
    error_message = "A disk_type=${var.disk_type} cannot be used with machine_type=${var.machine_type}."
  }
}
