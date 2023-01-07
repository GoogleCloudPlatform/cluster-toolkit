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
  install_file = templatefile(
    "${path.module}/templates/ramble_install.tpl",
    {
      install_dir = var.install_dir
      ramble_url  = var.ramble_url
      ramble_ref  = var.ramble_ref
      chown_owner = var.chown_owner
      chgrp_group = var.chgrp_group
      chmod_mode  = var.chmod_mode
      log_file    = var.log_file
    }
  )
  ramble_install_runner = {
    "type"        = "ansible-local"
    "content"     = local.install_file
    "destination" = "ramble_install.yml"
  }

  command_file = templatefile(
    "${path.module}/templates/ramble_commands.tpl",
    {
      spack_path     = var.spack_path
      install_dir    = var.install_dir
      log_file       = var.log_file
      COMMANDS       = var.commands
      command_prefix = ""
    }
  )

  ramble_commands_runner = {
    "type"        = "ansible-local"
    "content"     = local.command_file
    "destination" = "ramble_commands.yml"
  }

  ramble_deps_runner = {
    "type"        = "ansible-local"
    "source"      = "${path.module}/scripts/install_ramble_deps.yml"
    "destination" = "install_ramble_deps.yml"
  }
}
