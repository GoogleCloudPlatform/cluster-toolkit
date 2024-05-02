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

  depends_on = [
    # Depend on controller network, as a best effort to avoid
    # subnetwork resourceInUseByAnotherResource error
    # NOTE: Can not use nodeset subnetworks as "A static list expression is required"
    var.subnetwork_self_link,
  ]
}
