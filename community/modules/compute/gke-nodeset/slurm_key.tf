# Copyright 2025 Google LLC
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

locals {
  network_storage = {
    server_ip             = var.slurm_controller_instance.network_interface[0].network_ip
    remote_mount          = "/slurm/key_distribution" # defined in /community/modules/scheduler/schedmd-slurm-gcp-v6-controller/modules/slurm_files/scripts/util.py
    client_install_runner = {}
    mount_runner          = {}
    fs_type               = ""
    local_mount           = ""
    mount_options         = ""
  }
}

module "slurm_key_pv" {
  source          = "../../../../modules/file-system/gke-persistent-volume"
  labels          = {}
  capacity_gib    = 1
  cluster_id      = var.cluster_id
  filestore_id    = "projects/empty/locations/empty/instances/empty" # this does not apply since this NFS is not a filestore
  namespace       = var.slurm_namespace
  network_storage = local.network_storage
  pv_name         = "slurm-key-pv"
  pvc_name        = "slurm-key-pvc"
}
