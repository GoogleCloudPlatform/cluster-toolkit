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
  instance_template = var.instance_template != null ? var.instance_template : one(module.instance_template[*].self_link)

  tasks_per_node = var.task_count_per_node != null ? var.task_count_per_node : (var.mpi_mode ? 1 : null)

  job_template_contents = templatefile(
    "${path.module}/templates/batch-job-base.json.tftpl",
    {
      synchronized       = var.mpi_mode
      runnable           = var.runnable
      task_count         = var.task_count
      tasks_per_node     = local.tasks_per_node
      require_hosts_file = var.mpi_mode
      permissive_ssh     = var.mpi_mode
      log_policy         = var.log_policy
      instance_template  = local.instance_template
      nfs_volumes        = local.native_batch_network_storage
    }
  )

  job_id                   = var.job_id != null ? var.job_id : var.deployment_name
  job_filename             = var.job_filename != null ? var.job_filename : "cloud-batch-${local.job_id}.json"
  job_template_output_path = "${path.root}/${local.job_filename}"

  subnetwork_name    = var.subnetwork != null ? var.subnetwork.name : "default"
  subnetwork_project = var.subnetwork != null ? var.subnetwork.project : var.project_id

  # Filter network_storage for native Batch support
  native_batch_fstype = var.native_batch_mounting ? ["nfs"] : []
  native_batch_network_storage = [
    for ns in var.network_storage :
    ns if contains(local.native_batch_fstype, ns.fs_type)
  ]
  startup_script_network_storage = [
    for ns in var.network_storage :
    ns if !contains(local.native_batch_fstype, ns.fs_type)
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
}

module "batch_job_startup_script" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=v1.7.0"

  labels          = var.labels
  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region
  runners         = local.full_runner_list
}

module "instance_template" {
  source  = "terraform-google-modules/vm/google//modules/instance_template"
  version = "> 7.6.0"
  count   = var.instance_template == null ? 1 : 0

  name_prefix        = "${local.job_id}-instance-template"
  project_id         = var.project_id
  subnetwork         = local.subnetwork_name
  subnetwork_project = local.subnetwork_project
  service_account    = var.service_account
  access_config      = var.enable_public_ips ? [{ nat_ip = null, network_tier = null }] : []
  labels             = var.labels

  machine_type         = var.machine_type
  startup_script       = module.batch_job_startup_script.startup_script
  metadata             = var.network_storage != null ? ({ network_storage = jsonencode(var.network_storage) }) : {}
  source_image_family  = var.image.family
  source_image_project = var.image.project
}

resource "local_file" "job_template" {
  content  = local.job_template_contents
  filename = local.job_template_output_path
}
