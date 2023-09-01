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


module "slurm_cluster" {
  source = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster?ref=6.1.1"

  project_id                         = var.project_id
  slurm_cluster_name                 = var.slurm_cluster_name
  controller_instance_config         = var.controller_instance_config
  enable_hybrid                      = var.enable_hybrid
  controller_hybrid_config           = var.controller_hybrid_config
  login_nodes                        = var.login_nodes
  partitions                         = var.partitions
  enable_devel                       = var.enable_devel
  enable_cleanup_compute             = var.enable_cleanup_compute
  enable_bigquery_load               = var.enable_bigquery_load
  cloud_parameters                   = var.cloud_parameters
  network_storage                    = var.network_storage
  login_network_storage              = var.login_network_storage
  slurmdbd_conf_tpl                  = var.slurmdbd_conf_tpl
  slurm_conf_tpl                     = var.slurm_conf_tpl
  cgroup_conf_tpl                    = var.cgroup_conf_tpl
  controller_startup_scripts         = var.controller_startup_scripts
  login_startup_scripts              = var.login_startup_scripts
  compute_startup_scripts            = var.compute_startup_scripts
  prolog_scripts                     = var.prolog_scripts
  epilog_scripts                     = var.epilog_scripts
  cloudsql                           = var.cloudsql
  region                             = var.region
  create_bucket                      = var.create_bucket
  bucket_name                        = var.bucket_name
  bucket_dir                         = var.bucket_dir
  enable_login                       = var.enable_login
  nodeset                            = var.nodeset
  nodeset_dyn                        = var.nodeset_dyn
  nodeset_tpu                        = var.nodeset_tpu
  disable_default_mounts             = var.disable_default_mounts
  compute_startup_scripts_timeout    = var.compute_startup_scripts_timeout
  controller_startup_scripts_timeout = var.controller_startup_scripts_timeout
  login_startup_scripts_timeout      = var.login_startup_scripts_timeout
}
