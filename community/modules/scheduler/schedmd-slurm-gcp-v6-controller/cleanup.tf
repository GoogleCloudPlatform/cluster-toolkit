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

locals {
  cleanup_dependencies_agg = flatten([
    [
      for ns in var.nodeset : [
        ns.subnetwork_self_link,
        [for an in ns.additional_networks : an.subnetwork]
      ]
    ],
    [for ns in var.nodeset_tpu : ns.subnetwork],
  ])
}

resource "null_resource" "cleanup_compute_depenencies" {
  count = length(local.cleanup_dependencies_agg)
}

resource "null_resource" "cleanup_compute" {
  count = var.enable_cleanup_compute ? 1 : 0

  triggers = {
    project_id   = var.project_id
    cluster_name = local.slurm_cluster_name
  }

  provisioner "local-exec" {
    command = "/bin/bash ${path.module}/scripts/cleanup_compute.sh ${self.triggers.project_id} ${self.triggers.cluster_name}"
    when    = destroy
  }

  # Ensure that clean up is done before attempt to delete the networks
  depends_on = [null_resource.cleanup_compute_depenencies]
}
