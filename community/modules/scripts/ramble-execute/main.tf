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
  commands_content = var.commands == null ? "" : indent(4, yamlencode(var.commands))

  execute_contents = templatefile(
    "${path.module}/templates/ramble_execute.yml.tpl",
    {
      pre_script = ". ${var.spack_path}/share/spack/setup-env.sh && . ${var.ramble_path}/share/ramble/setup-env.sh"
      log_file   = var.log_file
      commands   = local.commands_content
    }
  )

  previous_ramble_runner_content = var.ramble_runner == null ? "" : var.ramble_runner["content"]

  runner_content = <<-EOT
    ${local.previous_ramble_runner_content}
    ${local.execute_contents}
  EOT

  execute_md5 = substr(md5(local.execute_contents), 0, 4)

  ramble_execute_runner = {
    "type"        = "ansible-local"
    "content"     = local.runner_content
    "destination" = "ramble_execute_${local.execute_md5}.yml"
  }
}

resource "local_file" "debug_file" {
  content  = local.execute_contents
  filename = "${path.module}/execute_script.yaml"
}
