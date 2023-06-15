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
  setup_file = templatefile(
    "${path.module}/templates/ramble_setup.yml.tpl",
    {
      install_dir = var.install_dir
      ramble_url  = var.ramble_url
      ramble_ref  = var.ramble_ref
      chown_owner = var.chown_owner == null ? "" : var.chown_owner
      chgrp_group = var.chgrp_group == null ? "" : var.chgrp_group
      chmod_mode  = var.chmod_mode == null ? "" : var.chmod_mode
    }
  )

  deps_file = templatefile(
    "${path.module}/templates/install_ramble_deps.yml.tpl",
    {
      ramble_ref = var.ramble_ref
    }
  )

  ramble_runner_content = <<-EOT
   ${local.setup_file}
   ${local.deps_file}
  EOT

  ramble_setup_runner = {
    "type"        = "ansible-local"
    "content"     = local.ramble_runner_content
    "destination" = "ramble_setup.yml"
  }
}
