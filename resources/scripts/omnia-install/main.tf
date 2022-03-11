/**
 * Copyright 2021 Google LLC
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
  inventory = templatefile(
    "${path.module}/templates/inventory.tpl",
    {
      omnia_manager = var.manager_ips
      omnia_compute = var.compute_ips
    }
  )
  add_user_file = templatefile(
    "${path.module}/templates/add_omnia_user.tpl",
    { username = var.omnia_username }
  )
  install_file = templatefile(
    "${path.module}/templates/install_omnia.tpl",
    {
      username    = var.omnia_username
      install_dir = var.install_dir
    }
  )
  create_omnia_install_dir_runner = {
    "type"        = "shell"
    "content"     = "mkdir ${var.install_dir}/omnia"
    "destination" = "mkdir-omnia.sh"
  }
  inventory_data_runner = {
    "type"        = "data"
    "content"     = local.inventory
    "destination" = "${var.install_dir}/omnia/inventory"
  }
  add_omnia_user_runner = {
    "type"        = "ansible-local"
    "content"     = local.add_user_file
    "destination" = "add_omnia_user.yml"
  }
  install_omnia_runner = {
    "type"        = "ansible-local"
    "content"     = local.install_file
    "destination" = "install_omnia.yml"
  }
}
