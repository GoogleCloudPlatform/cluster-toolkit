/**
 * Copyright 2022 Google LLC
 * Copyright (C) SchedMD LLC.
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
  # Default to value in partition_conf if both set the same key
  partition_conf = merge({
    "Default"     = var.is_default ? "YES" : null,
    "SuspendTime" = "INFINITE"
  }, var.partition_conf)

  # Since deployment name may be used to create a cluster name, we remove any invalid character from the beginning
  # Also, slurm imposed a lot of restrictions to this name, so we format it to an acceptable string
  tmp_cluster_name   = substr(replace(lower(var.deployment_name), "/^[^a-z]*|[^a-z0-9]/", ""), 0, 10)
  slurm_cluster_name = var.slurm_cluster_name != null ? var.slurm_cluster_name : local.tmp_cluster_name
}

module "slurm_partition" {
  source = "github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_partition?ref=5.12.0"

  slurm_cluster_name   = local.slurm_cluster_name
  enable_job_exclusive = var.exclusive
  partition_conf       = local.partition_conf
  partition_feature    = var.partition_feature
  partition_name       = var.partition_name
  partition_nodes      = []
  project_id           = var.project_id
  # region, subnetwork, and subnetwork_project do nothing in this configuration
  # but are currently required by the module
  region             = var.region
  subnetwork         = var.subnetwork_self_link == null ? "" : var.subnetwork_self_link
  subnetwork_project = var.subnetwork_project
}
