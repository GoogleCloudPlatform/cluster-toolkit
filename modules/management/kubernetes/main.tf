/**
 * Copyright 2024 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

locals {
  cluster_id_parts = split("/", var.cluster_id)
  cluster_name     = local.cluster_id_parts[5]
  cluster_location = local.cluster_id_parts[3]
  project_id       = var.project_id != null ? var.project_id : local.cluster_id_parts[1]

  yaml_separator = "\n---"

  # This locals block processes manifest inputs from one of four methods,
  # evaluated in order of precedence using coalesce.
  # --- METHOD 1: Direct Content Input ---
  content_yaml_body = var.content

  # Fallback for safe path checking in subsequent methods.
  null_safe_source = coalesce(var.source_path, " ")

  # --- METHOD 2: Single Local YAML File ---
  yaml_file         = length(regexall("\\.yaml(_.*)?$", lower(local.null_safe_source))) == 1 ? abspath(var.source_path) : null
  yaml_file_content = local.yaml_file != null ? file(local.yaml_file) : null

  # --- METHOD 3: Single Local Template File ---
  template_file         = length(regexall("\\.tftpl(_.*)?$", lower(local.null_safe_source))) == 1 ? abspath(var.source_path) : null
  template_file_content = local.template_file != null ? templatefile(local.template_file, var.template_vars) : null
  # --- METHOD 4: Directory of Files ---
  # Coalesce finds the first non-null content from the methods above.
  # The empty string "" prevents the function from failing if all are null.
  single_source_yaml_body = coalesce(local.content_yaml_body, local.yaml_file_content, local.template_file_content, " ")

  # Split the resulting YAML body into individual documents.
  single_source_docs_list = local.single_source_yaml_body != "" ? [for doc in split(local.yaml_separator, local.single_source_yaml_body) : trimspace(doc) if length(trimspace(doc)) > 0] : []

  # A directory is processed only if no single source was found AND the path ends with a slash.
  is_directory        = length(local.single_source_docs_list) == 0 && endswith(local.null_safe_source, "/")
  directory_path      = local.is_directory ? abspath(var.source_path) : null
  directory_yamls     = local.is_directory ? [for f in fileset(local.directory_path, "*.yaml") : file("${local.directory_path}/${f}")] : []
  directory_templates = local.is_directory ? [for f in fileset(local.directory_path, "*.tftpl") : templatefile("${local.directory_path}/${f}", var.template_vars)] : []
  # Corrected logic: Use concat to merge the results of the two for loops
  directory_docs_list = flatten(concat(
    [for yaml_content in local.directory_yamls : [for doc in split(local.yaml_separator, yaml_content) : trimspace(doc) if length(trimspace(doc)) > 0]],
    [for tpl_content in local.directory_templates : [for doc in split(local.yaml_separator, tpl_content) : trimspace(doc) if length(trimspace(doc)) > 0]]
  ))

  # --- FINAL AGGREGATION ---
  # Combine documents from both methods into a single map for the resource.
  all_docs_list = concat(local.single_source_docs_list, local.directory_docs_list)
  docs_map      = tomap({ for index, doc in local.all_docs_list : tostring(index) => doc })
}

provider "kubernetes" {
  host  = "https://${data.google_container_cluster.gke_cluster.endpoint}"
  token = data.google_client_config.default.access_token
  cluster_ca_certificate = base64decode(
    data.google_container_cluster.gke_cluster.master_auth[0].cluster_ca_certificate,
  )
}

data "google_client_config" "default" {}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

# Applies each manifest using the official hashicorp/kubernetes provider resource.
# This replaces the old `resource "kubectl_manifest"`.
resource "kubernetes_manifest" "this" {
  for_each = local.docs_map
  manifest = yamldecode(each.value)
  wait {
    rollout = true
  }
}
