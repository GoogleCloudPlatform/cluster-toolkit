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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "ramble-execute", ghpc_role = "scripts" })
}

locals {
  commands_content = var.commands == null ? "echo 'no ramble commands provided'" : indent(4, yamlencode(var.commands))

  execute_contents = templatefile(
    "${path.module}/templates/ramble_execute.yml.tpl",
    {
      pre_script = "if [ -f ${var.spack_profile_script_path} ]; then . ${var.spack_profile_script_path}; fi; . ${var.ramble_profile_script_path}"
      log_file   = var.log_file
      commands   = local.commands_content
    }
  )

  data_runners = [for data_file in var.data_files : merge(data_file, { type = "data" })]

  execute_md5 = substr(md5(local.execute_contents), 0, 4)
  execute_runner = {
    type        = "ansible-local"
    content     = local.execute_contents
    destination = "ramble_execute_${local.execute_md5}.yml"
  }

  previous_runners = var.ramble_runner != null ? [var.ramble_runner] : []
  runners          = concat(local.previous_runners, local.data_runners, [local.execute_runner])

  # Destinations should be unique while also being known at time of apply
  combined_unique_string = join("\n", [for runner in local.runners : runner["destination"]])
  combined_md5           = substr(md5(local.combined_unique_string), 0, 4)
  combined_runner = {
    type        = "shell"
    content     = module.startup_script.startup_script
    destination = "combined_install_ramble_${local.combined_md5}.sh"
  }
}

module "startup_script" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=50644b2"

  labels          = local.labels
  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region
  runners         = local.runners
  gcs_bucket_path = var.gcs_bucket_path
}

resource "local_file" "debug_file_ansible_execute" {
  content  = local.execute_contents
  filename = "${path.module}/debug_execute_${local.execute_md5}.yml"
}
