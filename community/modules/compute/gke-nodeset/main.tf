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


### GKE NodeSet
locals {
  machine_type_templates = ["a3-megagpu-8g", "a3-ultragpu-8g", "a4-highgpu-8g"]

  gpu_type      = var.has_gpu ? var.guest_accelerator[0].type : ""
  template_type = contains(local.machine_type_templates, var.machine_type) ? var.machine_type : (var.has_gpu ? "gpu-general" : var.has_tpu ? "tpu-general" : "general")
  manifest_path = "${path.module}/templates/nodeset-${local.template_type}.yaml.tftpl"
}

module "kubectl_apply" {
  source = "../../../../modules/management/kubectl-apply"

  cluster_id = var.cluster_id
  project_id = var.project_id

  apply_manifests = [{
    source = local.manifest_path,
    template_vars = {
      slurm_namespace    = var.slurm_namespace,
      nodeset_name       = "${var.slurm_cluster_name}-${var.nodeset_name}",
      nodeset_cr_name    = "${var.slurm_cluster_name}-${var.nodeset_name}",
      controller_name    = "${var.slurm_cluster_name}-controller",
      node_pool_name     = var.node_pool_names[0],
      node_count         = var.node_count,
      image              = var.image,
      gpu_per_node       = var.allocatable_gpu_per_node,
      home_pvc           = var.pvc_name,
      gpu_type           = local.gpu_type,
      tpu_chips_per_node = var.tpu_chips_per_node,
      tpu_accelerator    = var.tpu_accelerator,
      tpu_topology       = var.tpu_topology,
    }
  }]
}

data "google_storage_bucket" "this" {
  name = var.slurm_bucket_name

  depends_on = [var.slurm_bucket]
}

### Slurm NodeSet
locals {
  nodeset = {
    nodeset_name      = var.nodeset_name
    node_count_static = var.node_count
    subnetwork        = "https://www.googleapis.com/compute/v1/projects/${var.project_id}/regions/${var.subnetwork.region}/subnetworks/${var.subnetwork.name}"
    instance_template = var.instance_templates[0]
  }
}

resource "google_storage_bucket_object" "gke_nodeset_config" {
  bucket  = data.google_storage_bucket.this.name
  name    = "${var.slurm_bucket_dir}/nodeset_configs/${var.nodeset_name}.yaml"
  content = yamlencode(local.nodeset)
}
