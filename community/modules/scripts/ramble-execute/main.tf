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
  execute_file = templatefile(
    "${path.module}/templates/ramble_execute.tpl",
    {
      spack_path     = var.spack_path
      ramble_path    = var.ramble_path
      log_file       = var.log_file
      COMMANDS       = var.commands
      command_prefix = ""
    }
  )

  previous_ramble_runner_content = var.ramble_runner == null ? "" : var.ramble_runner["content"]

  runner_content = <<-EOT
    ${local.previous_ramble_runner_content}
    ${local.execute_file}
  EOT

  ramble_execute_runner = {
    "type"        = "ansible-local"
    "content"     = local.runner_content
    "destination" = "ramble_execute.yml"
  }
}
