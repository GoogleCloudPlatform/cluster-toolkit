/**
  * Copyright 2024 Google LLC
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
  yaml_separator = "\n---"

  content_yaml_body = var.content

  null_safe_source = coalesce(var.source_path, " ")

  url         = startswith(local.null_safe_source, "http://") || startswith(local.null_safe_source, "https://") ? var.source_path : null
  url_content = local.url != null ? data.http.yaml_content[0].response_body : null

  yaml_file         = local.url == null && length(regexall("\\.yaml(_.*)?$", lower(local.null_safe_source))) == 1 ? abspath(var.source_path) : null
  yaml_file_content = local.yaml_file != null ? file(local.yaml_file) : null

  template_file         = local.url == null && length(regexall("\\.tftpl(_.*)?$", lower(local.null_safe_source))) == 1 ? abspath(var.source_path) : null
  template_file_content = local.template_file != null ? templatefile(local.template_file, var.template_vars) : null

  yaml_body      = coalesce(local.content_yaml_body, local.url_content, local.yaml_file_content, local.template_file_content, " ")
  yaml_body_docs = [for doc in split(local.yaml_separator, local.yaml_body) : trimspace(doc) if length(trimspace(doc)) > 0] # Split yaml to single docs because the kubectl provider only supports single resource

  directory = length(local.yaml_body_docs) == 0 && endswith(local.null_safe_source, "/") ? abspath(var.source_path) : null

  docs_list = concat(try(local.yaml_body_docs, []), try(data.kubectl_path_documents.yamls[0].documents, []), try(data.kubectl_path_documents.templates[0].documents, []))
  docs_map = tomap({
    for index, doc in local.docs_list : index => doc
  })
}

data "http" "yaml_content" {
  count = local.url != null ? 1 : 0
  url   = local.url
}

data "kubectl_path_documents" "yamls" {
  count   = local.directory != null ? 1 : 0
  pattern = "${local.directory}/*.yaml"
}

data "kubectl_path_documents" "templates" {
  count   = local.directory != null ? 1 : 0
  pattern = "${local.directory}/*.tftpl"
  vars    = var.template_vars
}

resource "kubectl_manifest" "apply_doc" {
  for_each          = local.docs_map
  yaml_body         = each.value
  server_side_apply = var.server_side_apply
  wait_for_rollout  = var.wait_for_rollout
}
