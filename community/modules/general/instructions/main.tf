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
  raw_instructions = (
    var.template_path != null
    ? templatefile(var.template_path, var.template_vars)
    : coalesce(var.content, "")
  )

  banner_header = var.banner_title != null ? format("\n====================================================================\n==== %s ====\n====================================================================\n", var.banner_title) : ""

  banner_footer = var.banner_title != null ? "\n====================================================================\n" : ""

  formatted_instructions = (
    var.banner_title != null
    ? "${local.banner_header}${trimspace(local.raw_instructions)}${local.banner_footer}"
    : local.raw_instructions
  )

  file_destination = (
    var.write_to_file == null ? null : (
      startswith(var.write_to_file, "/")
      ? var.write_to_file
      : "${path.root}/${var.write_to_file}"
    )
  )
}

resource "local_file" "instructions_file" {
  count    = local.file_destination != null ? 1 : 0
  content  = local.formatted_instructions
  filename = local.file_destination
}

resource "terraform_data" "print_instructions" {
  count            = var.print_to_terminal ? 1 : 0
  input            = local.formatted_instructions
  triggers_replace = local.formatted_instructions

  provisioner "local-exec" {
    command = "printf '%s\\n' \"$INSTRUCTIONS\""
    environment = {
      INSTRUCTIONS = local.formatted_instructions
    }
  }
}
