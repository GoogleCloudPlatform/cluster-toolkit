/**
 * Copyright 2024 Google LLC
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
  pool_name = "${var.deployment_name}-pool"
  pool_duration = var.pool_duration != null ? var.pool_duration : "1h"
  machine_type = var.machine_type != null ? var.machine_type : "c2-standard-60"
  boot_image = var.boot_image != null ? var.boot_image : "batch-hpc-centos"
  make_mpi_pool_job_config_contents = templatefile(
    "${path.module}/templates/batch-make-mpi-pool-job-config.json.tftpl",
    {
      pool_name     = local.pool_name
      pool_size     = var.pool_size
      pool_duration = local.pool_duration
      machine_type  = local.machine_type
      boot_image    = local.boot_image
      nfs_share     = var.nfs_share
    }
  )
  run_mpi_workload_config_contents = templatefile(
    "${path.module}/templates/batch-run-mpi-workload.json.tftpl",
    {
      pool_name = local.pool_name
      pool_size = var.pool_size    
      nfs_share = var.nfs_share
    }
  )

  run_mpi_workload_config_path = "${path.root}/run-mpi-batch-job.json"
  make_pool_job_id = "${var.deployment_name}-make-pool"
  readme_contents = templatefile(
    "${path.module}/templates/readme.md.tftpl",
    {
      project = var.project_id
      location = var.region
      job_id = local.make_pool_job_id
      run_mpi_config_path = local.run_mpi_workload_config_path
    }
  )

  make_mpi_pool_job_config_path = "${path.root}/make-pool.json"
  make_pool_script_contents = templatefile(
    "${path.module}/templates/make-pool.sh.tftpl",
    {
      project = var.project_id
      location = var.region
      job_id = local.make_pool_job_id
      config = local.make_mpi_pool_job_config_path
    }
  )
}

resource "local_file" "make_mpi_pool_job_config" {
  content  = local.make_mpi_pool_job_config_contents
  filename = local.make_mpi_pool_job_config_path
}

resource "local_file" "run_mpi_workload_job_config" {
  content = local.run_mpi_workload_config_contents
  filename = local.run_mpi_workload_config_path
}

resource "local_file" "make_pool_script" {
  content = local.make_pool_script_contents
  filename = "make-pool.sh"
}

resource "local_file" "readme" {
  content = local.readme_contents
  filename = "README.md"
}

resource "null_resource" "run_make_pool" {
 depends_on = [ local_file.make_pool_script ]
 provisioner "local-exec" {
    command = "${path.root}/make-pool.sh"
  }
}
