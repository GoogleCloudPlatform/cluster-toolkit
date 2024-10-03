/**
 * Copyright 2024 Google LLC
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

locals {
  operation_instructions = <<-EOT
    Data is being imported from GCS bucket to parallelstore instance. It may
    not be available immediately.
  EOT
}

output "network_storage" {
  description = "Describes a parallelstore instance."
  value = {
    server_ip             = local.server_ip
    remote_mount          = local.remote_mount
    local_mount           = var.local_mount
    fs_type               = local.fs_type
    mount_options         = var.mount_options
    client_install_runner = local.client_install_runner
    mount_runner          = local.mount_runner
  }

  precondition {
    condition     = var.import_gcs_bucket_uri != null || var.import_destination_path == null
    error_message = <<-EOD
      Please specify import_gcs_bucket_uri to import data to parallelstore instance.
    EOD
  }
}

output "instructions" {
  description = "Instructions to monitor import-data operation from GCS bucket to parallelstore."
  value       = var.import_gcs_bucket_uri != null ? local.operation_instructions : "Data is not imported from GCS bucket."
}
