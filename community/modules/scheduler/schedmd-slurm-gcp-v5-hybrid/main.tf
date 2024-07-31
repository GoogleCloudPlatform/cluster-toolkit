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
  ghpc_startup_script_compute = [{
    filename = "ghpc_startup.sh"
    content  = var.compute_startup_script
  }]

  # Since deployment name may be used to create a cluster name, we remove any invalid character from the beginning
  # Also, slurm imposed a lot of restrictions to this name, so we format it to an acceptable string
  tmp_cluster_name   = substr(replace(lower(var.deployment_name), "/^[^a-z]*|[^a-z0-9]/", ""), 0, 10)
  slurm_cluster_name = var.slurm_cluster_name != null ? var.slurm_cluster_name : local.tmp_cluster_name

}

module "slurm_controller_instance" {
  source = "github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_controller_hybrid?ref=5.12.0"

  project_id                      = var.project_id
  slurm_cluster_name              = local.slurm_cluster_name
  enable_devel                    = var.enable_devel
  enable_cleanup_compute          = var.enable_cleanup_compute
  enable_cleanup_subscriptions    = var.enable_cleanup_subscriptions
  enable_reconfigure              = var.enable_reconfigure
  enable_bigquery_load            = var.enable_bigquery_load
  enable_slurm_gcp_plugins        = var.enable_slurm_gcp_plugins
  compute_startup_scripts         = local.ghpc_startup_script_compute
  compute_startup_scripts_timeout = var.compute_startup_scripts_timeout
  prolog_scripts                  = var.prolog_scripts
  epilog_scripts                  = var.epilog_scripts
  network_storage                 = var.network_storage
  disable_default_mounts          = var.disable_default_mounts
  login_network_storage           = var.network_storage
  partitions                      = var.partition
  google_app_cred_path            = var.google_app_cred_path
  slurm_bin_dir                   = var.slurm_bin_dir
  slurm_log_dir                   = var.slurm_log_dir
  cloud_parameters                = var.cloud_parameters
  output_dir                      = var.output_dir
  slurm_control_host              = var.slurm_control_host
  slurm_control_host_port         = var.slurm_control_host_port
  slurm_control_addr              = var.slurm_control_addr
  install_dir                     = var.install_dir
  munge_mount                     = var.munge_mount
}
