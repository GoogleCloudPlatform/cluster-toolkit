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

  partition_nodes = [
    {
      # Group Definition
      group_name    = "test"
      count_dynamic = var.count_dynamic
      count_static  = var.count_static
      node_conf     = {}

      # Template By Definition
      additional_disks       = []
      can_ip_forward         = false
      disable_smt            = false
      disk_auto_delete       = true
      disk_labels            = var.labels
      disk_size_gb           = var.disk_size_gb
      disk_type              = var.disk_type
      enable_confidential_vm = false
      enable_oslogin         = true
      enable_shielded_vm     = false
      gpu                    = var.gpu
      labels                 = {}
      machine_type           = var.machine_type
      metadata               = {}
      min_cpu_platform       = var.min_cpu_platform
      on_host_maintenance    = null
      preemptible            = var.preemptible
      service_account = {
        email = "default"
        scopes = [
          "https://www.googleapis.com/auth/cloud-platform",
        ]
      }
      shielded_instance_config = null
      source_image_family      = null
      source_image_project     = var.source_image_project
      source_image             = var.source_image
      tags                     = []

      # Template By Source
      instance_template = null
    },
  ]
}


module "slurm_partition" {
  source = "git::https://gitlab.com/SchedMD/slurm-gcp.git//terraform/modules/slurm_partition?ref=dev-v5"

  slurm_cluster_name      = var.slurm_cluster_name
  partition_nodes         = local.partition_nodes
  enable_job_exclusive    = var.exclusive
  enable_placement_groups = var.enable_placement
  network_storage         = var.network_storage
  partition_name          = var.partition_name
  project_id              = var.project_id
  region                  = var.region
  slurm_cluster_id        = "placeholder"
  subnetwork              = var.subnetwork_self_link
  partition_conf = {
    Default = "YES"
  }
}

