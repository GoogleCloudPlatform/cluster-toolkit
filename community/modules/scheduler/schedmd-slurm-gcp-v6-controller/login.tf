# Copyright 2025 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


locals {
  # TODO: deprecate `var.login_[ startup_script, startup_scripts_timeout, network_storage]`
  # in favour of vars defined in user-facing login module
  ghpc_startup_login = [{
    filename = "ghpc_startup.sh"
    content  = var.login_startup_script
  }]

  login_startup_scripts = concat(local.common_scripts, local.ghpc_startup_login)
}

module "login" {
  source   = "../../internal/slurm-gcp/login"
  for_each = { for x in var.login_nodes : x.group_name => x }

  project_id = var.project_id

  slurm_cluster_name = local.slurm_cluster_name
  slurm_bucket_path  = module.slurm_files.slurm_bucket_path
  slurm_bucket_name  = module.slurm_files.bucket_name
  slurm_bucket_dir   = module.slurm_files.bucket_dir

  login_nodes = each.value

  startup_scripts         = local.login_startup_scripts
  startup_scripts_timeout = var.login_startup_scripts_timeout

  network_storage = var.login_network_storage

  universe_domain = var.universe_domain

  # trigger replacement of login nodes when the controller instance is replaced
  # Needed for re-mounting volumes hosted on controller
  replace_trigger = var.enable_hybrid ? null : google_compute_instance_from_template.controller[0].self_link
}
