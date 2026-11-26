/**
 * Copyright 2026 Google LLC
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

locals {
  cluster_id_parts = split("/", var.cluster_id)
  project_id       = var.project_id != null && var.project_id != "" ? var.project_id : local.cluster_id_parts[1]

  templated_manifest = length(var.compute_classes) > 0 ? templatefile("${path.module}/templates/compute-class.yaml.tftpl", {
    compute_classes = var.compute_classes
  }) : ""

  custom_manifest = var.custom_manifest_path != null && var.custom_manifest_path != "" ? (
    endswith(var.custom_manifest_path, ".tftpl") || length(var.custom_template_vars) > 0 ?
    templatefile(var.custom_manifest_path, var.custom_template_vars) :
    file(var.custom_manifest_path)
  ) : ""

  combined_yaml = join("\n---\n", [for manifest in [local.templated_manifest, local.custom_manifest] : manifest if trimspace(manifest) != ""])

  manifest_docs     = [for doc in split("\n---", replace(replace(local.combined_yaml, "\r\n", "\n"), "/(?m)^---[ \t]*(?:#.*)?$/", "---")) : trimspace(doc) if length(trimspace(doc)) > 0]
  decoded_manifests = [for doc in local.manifest_docs : try(yamldecode(doc), null)]
  parsed_manifests = {
    for idx, doc in local.decoded_manifests : idx => doc if can(doc.apiVersion)
  }
}

module "kubectl_apply" {
  source = "../../../../modules/management/kubectl-apply"

  module_id              = var.module_id
  cluster_id             = var.cluster_id
  project_id             = local.project_id
  cluster_endpoint       = var.cluster_endpoint
  access_token           = var.access_token
  cluster_ca_certificate = var.cluster_ca_certificate

  apply_manifests = [
    for idx, doc in local.parsed_manifests : {
      content       = yamlencode(doc)
      template_vars = {}
    }
  ]
}
