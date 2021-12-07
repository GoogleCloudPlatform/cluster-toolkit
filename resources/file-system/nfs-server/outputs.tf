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
# render the content for each folder
output "network_storage" {
  description = "export of all desired folder directories"
  value = [for mount in var.local_mounts : {
    remote_mount  = "/exports${mount}"
    local_mount   = "/exports${mount}"
    fs_type       = "nfs"
    mount_options = "defaults,hard,intr"
    server_ip     = google_compute_instance.compute_instance.network_interface[0].network_ip
    }
  ]
}