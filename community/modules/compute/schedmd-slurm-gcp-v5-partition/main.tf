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
  ghpc_startup_script = [{
    filename = "ghpc_startup.sh"
    content  = var.startup_script
  }]

  # Default to value in partition_conf if both set "Default"
  partition_conf = merge(var.is_default == true ? { "Default" : "YES" } : {}, var.partition_conf)

  # Since deployment name may be used to create a cluster name, we remove any invalid character from the beginning
  # Also, slurm imposed a lot of restrictions to this name, so we format it to an acceptable string
  tmp_cluster_name   = substr(replace(lower(var.deployment_name), "/^[^a-z]*|[^a-z0-9]/", ""), 0, 10)
  slurm_cluster_name = var.slurm_cluster_name != null ? var.slurm_cluster_name : local.tmp_cluster_name

  all_zones      = toset(concat([var.zone], tolist(var.zones)))
  excluded_zones = [for z in data.google_compute_zones.available.names : z if !contains(local.all_zones, z)]

  reservation_map = { for x in var.node_groups : x.reservation_name => x if x.reservation_name != "" }
}

data "google_compute_zones" "available" {
  project = var.project_id
  region  = var.region
}

module "slurm_partition" {
  source = "github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_partition?ref=5.12.0"

  slurm_cluster_name                = local.slurm_cluster_name
  partition_nodes                   = var.node_groups
  enable_job_exclusive              = var.exclusive
  enable_placement_groups           = var.enable_placement
  enable_reconfigure                = var.enable_reconfigure
  network_storage                   = var.network_storage
  partition_name                    = var.partition_name
  project_id                        = var.project_id
  region                            = var.region
  zone_policy_allow                 = [] # this setting is effectively useless because allow is implied default
  zone_policy_deny                  = local.excluded_zones
  zone_target_shape                 = var.zone_target_shape
  subnetwork                        = var.subnetwork_self_link == null ? "" : var.subnetwork_self_link
  subnetwork_project                = var.subnetwork_project
  partition_conf                    = local.partition_conf
  partition_startup_scripts         = local.ghpc_startup_script
  partition_startup_scripts_timeout = var.partition_startup_scripts_timeout
}

# tflint-ignore: terraform_unused_declarations
data "google_compute_reservation" "reservation" {
  project = var.project_id
  zone    = var.zone

  for_each = local.reservation_map
  name     = each.value.reservation_name

  lifecycle {
    postcondition {
      condition     = self.self_link != null
      error_message = "couldn't find the reservation ${each.value.reservation_name}}"
    }

    postcondition {
      condition     = coalesce(self.specific_reservation_required, true)
      error_message = <<EOT
        your reservation has to be specific,
        see https://cloud.google.com/compute/docs/instances/reservations-overview#how-reservations-work
        for more information. if it's intentionally automatic, don't specify
        it in the blueprint.
      EOT
    }
  }
}
