# Copyright 2023 Google LLC
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

# locals {
#   # This label allows for billing report tracking based on module.
#   labels = merge(var.labels, { ghpc_module = "schedmd-slurm-gcp-v6-nodeset", ghpc_role = "compute" })
# }

locals {

  nodeset_tpu = {
    node_count_static      = var.node_count_static
    node_count_dynamic_max = var.node_count_dynamic_max
    nodeset_name           = var.name
    node_type              = var.node_type

    accelerator_config = var.accelerator_config
    tf_version         = var.tf_version
    preemptible        = var.preemptible
    preserve_tpu       = var.preserve_tpu

    data_disks   = var.data_disks
    docker_image = var.docker_image

    enable_public_ip = !var.disable_public_ips
    # TODO: rename to subnetwork_self_link, requires changes to the scripts
    subnetwork      = var.subnetwork_self_link
    service_account = var.service_account
    zone            = var.zone
  }
}
