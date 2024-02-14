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

# This file is meant to be reused by multiple modules.
# "inputs":
# local.native_fstype : list of file systems that are supported automatically, but looking at the metadata.
# var.network_storage : to be passed into metadata somewhere else (not here)
# var.startup_script  : to be changed into a more complete file system with all the fs runners

# "outputs":
# local.startup_from_network_storage : A full startup script with all the runners that are not supported
#                                      natively and were included in the network_storage structure

locals {
  startup_script_network_storage = [
    for ns in var.network_storage :
    ns if !contains(local.native_fstype, ns.fs_type)
  ]
  # Pull out runners to include in startup script
  storage_client_install_runners = [
    for ns in local.startup_script_network_storage :
    ns.client_install_runner if ns.client_install_runner != null
  ]
  mount_runners = [
    for ns in local.startup_script_network_storage :
    ns.mount_runner if ns.mount_runner != null
  ]

  startup_script_runner = [{
    content     = var.startup_script != null ? var.startup_script : "echo 'No user provided startup script.'"
    destination = "passed_startup_script.sh"
    type        = "shell"
  }]

  full_runner_list = concat(
    local.storage_client_install_runners,
    local.mount_runners,
    local.startup_script_runner
  )

  startup_from_network_storage = module.netstorage_startup_script.startup_script
}

module "netstorage_startup_script" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=50644b2"

  labels          = local.labels
  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region
  runners         = local.full_runner_list
}
