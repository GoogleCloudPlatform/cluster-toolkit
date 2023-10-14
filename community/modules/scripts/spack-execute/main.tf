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
  labels = merge(var.labels, { ghpc_module = "spack-execute", ghpc_role = "scripts" })
}

locals {
  commands_content = var.commands == null ? "echo 'no spack commands provided'" : indent(4, yamlencode(var.commands))

  execute_contents = templatefile(
    "${path.module}/templates/execute_commands.yml.tpl",
    {
      pre_script = ". /etc/profile.d/spack.sh"
      log_file   = var.log_file
      commands   = local.commands_content
    }
  )

  data_runners = [for data_file in var.data_files : merge(data_file, { type = "data" })]

  execute_md5 = substr(md5(local.execute_contents), 0, 4)
  execute_runner = {
    type        = "ansible-local"
    content     = local.execute_contents
    destination = "spack_execute_${local.execute_md5}.yml"
  }

  previous_runners = var.spack_runner != null ? [var.spack_runner] : []
  runners          = concat(local.previous_runners, local.data_runners, [local.execute_runner])

  # Destinations should be unique while also being known at time of apply
  combined_unique_string = join("\n", [for runner in local.runners : runner["destination"]])
  combined_md5           = substr(md5(local.combined_unique_string), 0, 4)
  combined_runner = {
    type        = "shell"
    content     = module.startup_script.startup_script
    destination = "combined_install_spack_${local.combined_md5}.sh"
  }

  bucket_md5  = substr(md5("${var.project_id}.${var.deployment_name}"), 0, 4)
  bucket_name = "ramble-scripts-${local.bucket_md5}-${local.combined_md5}"
}

resource "google_storage_bucket" "bucket" {
  count                       = var.gcs_bucket_path != null ? 0 : 1
  project                     = var.project_id
  name                        = local.bucket_name
  uniform_bucket_level_access = true
  location                    = var.region
  storage_class               = "REGIONAL"
  labels                      = local.labels
}

module "startup_script" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=v1.22.1"

  labels          = local.labels
  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region
  runners         = local.runners
  gcs_bucket_path = var.gcs_bucket_path != null ? var.gcs_bucket_path : "gs://${google_storage_bucket.bucket[0].name}"
}

resource "local_file" "debug_file_ansible_execute" {
  content  = local.execute_contents
  filename = "${path.module}/debug_execute_${local.execute_md5}.yml"
}
