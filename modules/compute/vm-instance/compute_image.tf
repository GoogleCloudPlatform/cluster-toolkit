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

data "google_compute_image" "compute_image" {
  family  = try(var.instance_image.family, null)
  name    = try(var.instance_image.name, null)
  project = try(var.instance_image.project, null)

  lifecycle {
    postcondition {
      # Condition needs to check the suffix of the license, as prefix contains an API version which can change.
      # Example license value: https://www.googleapis.com/compute/v1/projects/cloud-hpc-image-public/global/licenses/hpc-vm-image-feature-disable-auto-updates
      condition     = var.allow_automatic_updates || anytrue([for license in self.licenses : endswith(license, "/projects/cloud-hpc-image-public/global/licenses/hpc-vm-image-feature-disable-auto-updates")])
      error_message = "Disabling automatic updates is not supported with the selected VM image.  More information: https://cloud.google.com/compute/docs/instances/create-hpc-vm#disable_automatic_updates"
    }
  }
}
