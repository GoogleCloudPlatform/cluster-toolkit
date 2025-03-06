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

output "persistent_volume_claims" {
  description = "An object that describes a k8s PVC created by this module."
  value = flatten([
    for idx in range(var.pvc_count) : [{
      name          = "${local.pvc_name_prefix}-${idx}"
      mount_path    = "${var.pv_mount_path}/${local.pvc_name_prefix}-${idx}"
      mount_options = var.mount_options
      is_gcs        = false
    }]
  ])
}
