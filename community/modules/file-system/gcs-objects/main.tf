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
  # Expand local directories recursively using fileset, filtering by regex exclude patterns
  dir_objects = merge(concat([{}], [
    for d in var.directories : {
      for rel in fileset(startswith(d.source_path, "/") ? d.source_path : "${path.root}/${d.source_path}", "**") :
      trim("${trim(d.destination_path, "/")}/${rel}", "/") => {
        source_path  = startswith(d.source_path, "/") ? "${d.source_path}/${rel}" : "${path.root}/${d.source_path}/${rel}"
        content      = null
        content_type = null
      }
      if !anytrue([for pattern in coalesce(d.exclude, []) : can(regex(pattern, rel))])
    }
  ])...)

  # Map individual files
  file_objects = {
    for f in var.files :
    trim(f.destination_path, "/") => {
      source_path  = f.source_path != null ? (startswith(f.source_path, "/") ? f.source_path : "${path.root}/${f.source_path}") : null
      content      = f.content
      content_type = f.content_type
    }
  }

  all_objects = merge(local.dir_objects, local.file_objects)
}

resource "google_storage_bucket_object" "objects" {
  for_each = local.all_objects

  name           = each.key
  bucket         = var.bucket_name
  content        = each.value.content
  source         = each.value.source_path
  content_type   = each.value.content_type
  source_md5hash = each.value.content != null ? md5(each.value.content) : filemd5(each.value.source_path)

  timeouts {
    create = "10m"
    update = "10m"
  }
}
