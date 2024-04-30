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
  cleanup_compute_cmd = (
    var.enable_cleanup_compute ?
    "/bin/bash ${path.module}/scripts/cleanup_compute.sh ${var.project_id} ${local.slurm_cluster_name}" :
    "echo noop"
  )
}

resource "null_resource" "cleanup_compute" {
  triggers = {
    cmd = local.cleanup_compute_cmd
  }

  provisioner "local-exec" {
    command = self.triggers.cmd
    when    = destroy
  }

  lifecycle {
    # IMPORTANT: we make use of terraform bug https://github.com/hashicorp/terraform/issues/13549
    # Without this `create_before_destroy`, the provisioner would be executed on any change to 
    # `var.enable_cleanup_compute`, even (true -> false).
    # With this `create_before_destroy` the provisioner will be only executed during `destroy`.
    create_before_destroy = true
  }

  depends_on = [
    # Depend on controller network, as a best effort to avoid
    # subnetwork resourceInUseByAnotherResource error
    # NOTE: Can not use nodeset subnetworks as "A static list expression is required"
    var.subnetwork_self_link,
  ]
}
