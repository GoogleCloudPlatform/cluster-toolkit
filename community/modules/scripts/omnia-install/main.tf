/**
 * Copyright 2022 Google LLC
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
  install_dir = var.install_dir != "" ? var.install_dir : "/home/${var.omnia_username}"
  nodecount   = length(var.compute_ips) + length(var.manager_ips)
  inventory = templatefile(
    "${path.module}/templates/inventory.tpl",
    {
      omnia_manager = var.manager_ips
      omnia_compute = var.compute_ips
    }
  )
  setup_omnia_node_file = templatefile(
    "${path.module}/templates/setup_omnia_node.tpl",
    {
      username    = var.omnia_username
      install_dir = local.install_dir
    }
  )
  install_file = templatefile(
    "${path.module}/templates/install_omnia.tpl",
    {
      username      = var.omnia_username
      install_dir   = local.install_dir
      omnia_compute = var.compute_ips
      nodecount     = local.nodecount
      slurm_uid     = var.slurm_uid
    }
  )
  inventory_path = "${local.install_dir}/inventory"
  copy_inventory_runner = {
    "type"        = "data"
    "content"     = local.inventory
    "destination" = local.inventory_path
  }
  setup_omnia_node_runner = {
    "type"        = "ansible-local"
    "content"     = local.setup_omnia_node_file
    "destination" = "setup_omnia_node.yml"
  }
  install_omnia_runner = {
    "type"        = "ansible-local"
    "content"     = local.install_file
    "destination" = "install_omnia.yml"
  }
}
