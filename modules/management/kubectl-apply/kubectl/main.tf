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

  # This locals block processes manifest inputs from one of four methods,
  # evaluated in order of precedence using coalesce.

  # --- METHOD 1: Direct Content Input ---
  # Used when manifest content is passed directly as a string.
  content_yaml_body = var.content

  # Fallback for safe path checking in subsequent methods.
  null_safe_source = coalesce(var.source_path, " ")

  # --- METHOD 2: Single Local YAML File ---
  # Used when var.source_path points to a local .yaml file.
  yaml_file         = length(regexall("\\.yaml(_.*)?$", lower(local.null_safe_source))) == 1 ? abspath(var.source_path) : null
  yaml_file_content = local.yaml_file != null ? file(local.yaml_file) : null

  # --- METHOD 3: Single Local Template File ---
  # Used when var.source_path points to a local .tftpl file.
  template_file         = length(regexall("\\.tftpl(_.*)?$", lower(local.null_safe_source))) == 1 ? abspath(var.source_path) : null
  template_file_content = local.template_file != null ? templatefile(local.template_file, var.template_vars) : null

  # --- CONSOLIDATE & PROCESS ---
  # Coalesce finds the first non-null content from the methods above.
  yaml_body = coalesce(local.content_yaml_body, local.yaml_file_content, local.template_file_content, " ")
  # Ensure only valid YAML is processed
  # It explicitly tests if the content can be decoded before including it.
  yaml_body_docs = compact(flatten([
    for doc in split(local.yaml_separator, local.yaml_body) : [
      for content in [trimspace(doc)] : (
        # Use a temporary local variable and can() to test for successful YAML decoding.
        # This handles malformed documents (like comment blocks) which cause yamldecode() to fail.
        can(yamldecode(content)) && length(yamldecode(content)) > 0 ? content : null
      )
    ]
  ]))

  # --- METHOD 4: Directory of Files ---
  # If no content was found via the methods above AND the source path looks like a directory,
  # we assume this is the desired method. The data blocks below will handle it.
  directory = length(local.yaml_body_docs) == 0 && endswith(local.null_safe_source, "/") ? abspath(var.source_path) : null

  # --- FINAL AGGREGATION ---
  # Combine documents from single-source methods and directory-scan methods into one list.
  docs_list = concat(try(local.yaml_body_docs, []), try(data.kubectl_path_documents.yamls[0].documents, []), try(data.kubectl_path_documents.templates[0].documents, []))
  docs_map = tomap({
    for index, doc in local.docs_list : index => doc
  })
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
  force_conflicts   = var.force_conflicts

  lifecycle {
    precondition {
      condition     = !var.force_conflicts || var.server_side_apply
      error_message = "The 'force_conflicts' variable can only be set to true when 'server_side_apply' is also true."
    }
  }
}
