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
  staged_directories = [
    for d in var.directories : {
      source_path      = d.source_path
      destination_path = d.destination_path
      destination_uri  = trim(d.destination_path, "/") == "" ? "gs://${var.bucket_name}/" : "gs://${var.bucket_name}/${trim(d.destination_path, "/")}/"
      repo_url         = try(coalesce(d.repo_url, var.repo_url), "") == "local" ? "" : try(coalesce(d.repo_url, var.repo_url), "")
      repo_ref         = try(coalesce(d.repo_ref, var.repo_ref), "")
      exclude_regex    = join("|", d.exclude)
      content_hash = try(coalesce(d.repo_url, var.repo_url, "local"), "local") == "local" ? (
        try(
          sha256(jsonencode({
            for f in fileset(d.source_path, "**") : f => filesha256("${d.source_path}/${f}")
            if length([for ex in d.exclude : ex if can(regex(ex, f))]) == 0
          })),
          try(
            sha256(jsonencode({
              for f in fileset("${path.root}/${d.source_path}", "**") : f => filesha256("${path.root}/${d.source_path}/${f}")
              if length([for ex in d.exclude : ex if can(regex(ex, f))]) == 0
            })),
            try(
              sha256(jsonencode({
                for f in fileset("${path.root}/../../${d.source_path}", "**") : f => filesha256("${path.root}/../../${d.source_path}/${f}")
                if length([for ex in d.exclude : ex if can(regex(ex, f))]) == 0
              })),
              try(
                sha256(jsonencode({
                  for f in fileset("${path.root}/../../../${d.source_path}", "**") : f => filesha256("${path.root}/../../../${d.source_path}/${f}")
                  if length([for ex in d.exclude : ex if can(regex(ex, f))]) == 0
                })),
                ""
              )
            )
          )
        )
      ) : ""
    }
  ]

  staged_files = [
    for f in var.files : {
      source_path      = f.source_path
      destination_path = f.destination_path
      destination_uri = (
        endswith(f.destination_path, "/") || trim(f.destination_path, "/") == ""
        ? "gs://${var.bucket_name}/${trim(f.destination_path, "/") == "" ? "" : "${trim(f.destination_path, "/")}/"}${basename(f.source_path)}"
        : "gs://${var.bucket_name}/${trim(f.destination_path, "/")}"
      )
      repo_url = try(coalesce(f.repo_url, var.repo_url), "") == "local" ? "" : try(coalesce(f.repo_url, var.repo_url), "")
      repo_ref = try(coalesce(f.repo_ref, var.repo_ref), "")
      content_hash = try(coalesce(f.repo_url, var.repo_url, "local"), "local") == "local" ? (
        try(
          filesha256(f.source_path),
          try(
            filesha256("${path.root}/${f.source_path}"),
            try(
              filesha256("${path.root}/../../${f.source_path}"),
              try(filesha256("${path.root}/../../../${f.source_path}"), "")
            )
          )
        )
      ) : ""
    }
  ]
}

resource "terraform_data" "stage_gcs_directory" {
  for_each = {
    for d in local.staged_directories : "${d.source_path}->${d.destination_uri}" => d
  }

  triggers_replace = [
    var.bucket_name,
    each.value.source_path,
    each.value.repo_url,
    each.value.repo_ref,
    each.value.destination_uri,
    each.value.exclude_regex,
    each.value.content_hash,
  ]

  provisioner "local-exec" {
    interpreter = ["/bin/bash"]
    command     = "${path.module}/scripts/stage_to_gcs.sh"

    environment = {
      MODE          = "directory"
      SRC_PATH      = each.value.source_path
      DEST_URI      = each.value.destination_uri
      REPO_URL      = each.value.repo_url
      REPO_REF      = each.value.repo_ref
      EXCLUDE_REGEX = each.value.exclude_regex
    }
  }
}

resource "terraform_data" "stage_gcs_file" {
  for_each = {
    for f in local.staged_files : "${f.source_path}->${f.destination_uri}" => f
  }

  triggers_replace = [
    var.bucket_name,
    each.value.source_path,
    each.value.repo_url,
    each.value.repo_ref,
    each.value.destination_uri,
    each.value.content_hash,
  ]

  provisioner "local-exec" {
    interpreter = ["/bin/bash"]
    command     = "${path.module}/scripts/stage_to_gcs.sh"

    environment = {
      MODE     = "file"
      SRC_PATH = each.value.source_path
      DEST_URI = each.value.destination_uri
      REPO_URL = each.value.repo_url
      REPO_REF = each.value.repo_ref
    }
  }
}
