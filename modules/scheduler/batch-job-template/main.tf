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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "batch-job-template", ghpc_role = "scheduler" })
}

locals {
  instance_template = coalesce(var.instance_template, module.instance_template.self_link)

  tasks_per_node = coalesce(var.task_count_per_node, (var.mpi_mode ? 1 : null))

  job_template_contents = templatefile(
    "${path.module}/templates/batch-job-base.yaml.tftpl",
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

  job_id_base              = coalesce(var.job_id, var.deployment_name)
  submit_job_id            = "${local.job_id_base}-${random_id.submit_job_suffix.hex}"
  job_filename             = coalesce(var.job_filename, "cloud-batch-${local.job_id_base}.yaml")
  job_template_output_path = "${path.root}/${local.job_filename}"

  submit_script_contents = templatefile(
    "${path.module}/templates/batch-submit.sh.tftpl",
    {
      project       = var.project_id
      location      = var.region
      config        = local_file.job_template.filename
      submit_job_id = local.submit_job_id
    }
  )
  submit_script_output_path = "${path.root}/submit-job.sh"

  subnetwork_name    = var.subnetwork != null ? var.subnetwork.name : "default"
  subnetwork_project = var.subnetwork != null ? var.subnetwork.project : var.project_id

  # Filter network_storage for native Batch support
  native_fstype = var.native_batch_mounting ? ["nfs"] : []
  native_batch_network_storage = [
    for ns in var.network_storage :
    ns if contains(local.native_fstype, ns.fs_type)
  ]
  # other processing happens in startup_from_network_storage.tf

  # this code is similar to code in Packer and vm-instance modules
  # it differs in that this module does not (yet) expose var.guest_acclerator
  # for attaching GPUs to N1 VMs. For now, identify only A2 types.
  machine_vals                = split("-", var.machine_type)
  machine_family              = local.machine_vals[0]
  gpu_attached                = contains(["a2", "g2"], local.machine_family)
  on_host_maintenance_default = local.gpu_attached ? "TERMINATE" : "MIGRATE"

  on_host_maintenance = coalesce(var.on_host_maintenance, local.on_host_maintenance_default)
}

module "instance_template" {
  source  = "terraform-google-modules/vm/google//modules/instance_template"
  version = "~> 8.0"

  name_prefix        = var.instance_template == null ? "${local.job_id_base}-instance-template" : "unused-template"
  project_id         = var.project_id
  subnetwork         = local.subnetwork_name
  subnetwork_project = local.subnetwork_project
  service_account    = var.service_account
  access_config      = var.enable_public_ips ? [{ nat_ip = null, network_tier = null }] : []
  labels             = local.labels

  machine_type         = var.machine_type
  startup_script       = local.startup_from_network_storage
  metadata             = var.network_storage != null ? ({ network_storage = jsonencode(var.network_storage) }) : {}
  source_image_family  = try(var.instance_image.family, "")
  source_image         = try(var.instance_image.name, "")
  source_image_project = var.instance_image.project
  on_host_maintenance  = local.on_host_maintenance
}

resource "local_file" "job_template" {
  content  = local.job_template_contents
  filename = local.job_template_output_path
}

resource "random_id" "submit_job_suffix" {
  byte_length = 4
  keepers = {
    always_run = timestamp()
  }
}

resource "local_file" "submit_script" {
  content  = local.submit_script_contents
  filename = local.submit_script_output_path
}

resource "null_resource" "submit_job" {
  depends_on = [local_file.job_template]
  count      = var.submit ? 1 : 0

  # A new deployment should always submit a new job. Old finished jobs aren't persistent parts of
  # Cloud infrastructure.
  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = local.submit_script_output_path
  }
}
