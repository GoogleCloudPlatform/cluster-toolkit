# Copyright 2026 Google LLC
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
  manifest_path = "${path.module}/templates/nodeset-general.yaml.tftpl"
}

module "kubectl_apply" {
  source = "../../../../modules/management/kubectl-apply"

  cluster_id = var.cluster_id
  project_id = var.project_id

  apply_manifests = [{
    source = local.manifest_path,
    template_vars = {
      slurm_namespace = var.slurm_namespace,
      nodeset_name    = "${var.slurm_cluster_name}-${var.nodeset_name}",
      nodeset_cr_name = "${var.slurm_cluster_name}-${var.nodeset_name}",
      controller_name = "${var.slurm_cluster_name}-controller",
      node_pool_name  = var.node_pool_names[0],
      node_count      = var.node_count_static,
      image           = var.image,
      home_pvc        = module.home_pv.pvc_name
      slurm_key_pvc   = module.slurm_key_pv.pvc_name
    }
  }]
}

data "google_storage_bucket" "this" {
  name = var.slurm_bucket[0].name

  depends_on = [var.slurm_bucket]
}

### Slurm NodeSet
locals {
  nodeset = {
    gke_nodepool      = var.node_pool_names[0]
    nodeset_name      = var.nodeset_name
    node_count_static = var.node_count_static
    subnetwork        = "https://www.googleapis.com/compute/v1/projects/${var.project_id}/regions/${var.subnetwork.region}/subnetworks/${var.subnetwork.name}"
    instance_template = var.instance_templates[0]
  }
}

resource "google_storage_bucket_object" "gke_nodeset_config" {
  bucket  = data.google_storage_bucket.this.name
  name    = "${var.slurm_bucket_dir}/nodeset_configs/${var.nodeset_name}.yaml"
  content = yamlencode(local.nodeset)
}
