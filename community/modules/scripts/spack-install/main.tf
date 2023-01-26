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
    "${path.module}/templates/spack_install.tpl",
    {
      install_dir = var.install_dir
      spack_url   = var.spack_url
      spack_ref   = var.spack_ref
      chown_owner = var.chown_owner
      chgrp_group = var.chgrp_group
      chmod_mode  = var.chmod_mode
    }
  )

  spack_install_runner = {
    "type"        = "ansible-local"
    "content"     = <<-EOD
      ${local.install_file}
      EOD
    "destination" = "spack_install.yml"
  }

  command_file = templatefile(
    "${path.module}/templates/spack_commands.tpl",
    {
      install_dir = var.install_dir
      log_file    = var.log_file
      COMMANDS    = var.commands
    }
  )

  spack_commands_runner = {
    "type"        = "ansible-local"
    "content"     = <<-EOD
      ${local.command_file}
      EOD
    "destination" = "spack_commands.yml"
  }

  package_commands = [for pkg_spec in var.packages : "install ${pkg_spec}"]

  packages_file = templatefile(
    "${path.module}/templates/spack_commands.tpl",
    {
      install_dir = var.install_dir
      log_file    = var.log_file
      COMMANDS    = local.package_commands
    }
  )

  spack_packages_runner = {
    "type"        = "ansible-local"
    "content"     = <<-EOD
      ${local.packages_file}
      EOD
    "destination" = "spack_packages.yml"
  }

  compiler_commands = flatten([for comp_spec in var.compilers : ["install ${comp_spec}", "load ${comp_spec}", "compiler find --scope=site", "unload ${comp_spec}"]])

  compiler_file = templatefile(
    "${path.module}/templates/spack_commands.tpl",
    {
      install_dir = var.install_dir
      log_file    = var.log_file
      COMMANDS    = local.compiler_commands
    }
  )

  spack_compilers_runner = {
    "type"        = "ansible-local"
    "content"     = <<-EOD
      ${local.compiler_file}
      EOD
    "destination" = "spack_compilers.yml"
  }

  install_spack_runner = {
    "type"        = "ansible-local"
    "content"     = <<-EOD
      ${local.install_file}
      ${local.command_file}
      ${local.compiler_file}
      ${local.packages_file}
      EOD
    "destination" = "complete_spack_install.yml"
  }

  install_spack_deps_runner = {
    "type"        = "ansible-local"
    "source"      = "${path.module}/scripts/install_spack_deps.yml"
    "destination" = "install_spack_deps.yml"
  }
}
