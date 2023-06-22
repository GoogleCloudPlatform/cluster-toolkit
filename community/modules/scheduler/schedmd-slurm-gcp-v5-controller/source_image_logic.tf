/**
 * Copyright 2023 Google LLC
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
  # Handle VM image format from 2 sources, prioritize source_image* variables
  # over instance_image
  source_image_input_used = var.source_image != "" || var.source_image_family != "" || var.source_image_project != ""
  source_image            = local.source_image_input_used ? var.source_image : lookup(var.instance_image, "name", "")
  source_image_family     = local.source_image_input_used ? var.source_image_family : lookup(var.instance_image, "family", "")
  source_image_project    = local.source_image_input_used ? var.source_image_project : lookup(var.instance_image, "project", "")
  source_image_project_normalized = (
    local.source_image != "" || length(regexall("/", local.source_image_project)) > 0
    ? local.source_image_project
    : "projects/${local.source_image_project}/global/images/family"
  )
}
