# Copyright 2023 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This modules translate from the Toolkit format to the SchedMD format,
# the example of such translation is `guest_accelerator` to `gpu`.
#
# The variables during translation go through the following steps signaled by name suffix:
# 1. var.X - intake in the Toolkit format;
# 2. local.X - var.X OR default_X;
# 3. local.X_full - local.X augmented with default values, 
#      e.g. `network_storage` from `cluster` is propogated to `partition`.
# 4. local.X_out - local.X_full translated to the SchedMD format.

locals { # Nodeset
  nodeset_default = [{
    name         = "defaultns",
    machine_type = "n1-standard-4",
    # TODO: specify more
  }]
  # Use jsonencode/decode to workaround Terraform's types consistency check.
  nodeset = jsondecode(length(var.nodeset) == 0 ?
  jsonencode(local.nodeset_default) : jsonencode(var.nodeset))

  nodeset_full = [for ns in local.nodeset : merge(ns, {
    region               = coalesce(try(ns.region, null), var.region),
    subnetwork_self_link = coalesce(try(ns.subnetwork_self_link, null), var.subnetwork_self_link), # 
    instance_image       = coalesce(try(ns.instance_image, null), {}),
    # TODO: service_account, instance_image
  })]


  nodeset_out = [
    for ns in local.nodeset_full : merge(ns, {
      # Specify only fields different accross the formats, 
      # the rest will be merged straight from `ns`.
      nodeset_name = ns.name
      disable_smt  = !try(ns.enable_smt, false)
      gpu          = one(try(ns.guest_accelerator, []))

      source_image_family  = lookup(ns.instance_image, "family", "")
      source_image_project = lookup(ns.instance_image, "project", "") # TODO: use normalized project
      source_image         = lookup(ns.instance_image, "name", "")

      # subnetwork_project - omit as we use subnewtork self_link
      subnetwork = ns.subnetwork_self_link
      spot       = try(ns.enable_spot_vm, false)

      termination_action = try(ns.spot_instance_config.termination_action, null)
  })]
}

locals { # Partition
  partition_default = [{
    name              = "default"
    is_default        = true
    exclusive         = true
    partition_nodeset = [for ns in local.nodeset_out : ns.nodeset_name]
  }]
  # Use jsonencode/decode to workaround Terraform's types consistency check.
  partitions = jsondecode(length(var.partition) == 0 ? jsonencode(local.partition_default) : jsonencode(var.partition))
  partitions_full = [for p in local.partitions : merge(p, {
    # TODO: network_storage
  })]
  partitions_out = [
    for p in local.partitions_full : merge(p, {
      # Specify only fields different accross the formats, 
      # the rest will be merged straight from `p`.
      default              = p.is_default,
      enable_job_exclusive = p.exclusive,
      partition_name       = p.name,
    })
  ]
}

locals {                      # Controller
  controller = var.controller # var.controller default value is sufficient
  controller_full = merge(local.controller, {
    region               = coalesce(try(local.controller.region, null), var.region),
    subnetwork_self_link = coalesce(try(local.controller.subnetwork_self_link, null), var.subnetwork_self_link),
    instance_image       = coalesce(try(local.controller.instance_image, null), {}),
    # TODO: service_account, instance_image, network_storage
  })
  cf = local.controller_full # alias for shortness
  controller_out = merge(local.cf, {
    # Specify only fields different accross the formats, 
    # the rest will be merged straight from `controller_full`.
    disable_smt          = !try(local.cf.enable_smt, false)
    gpu                  = one(try(local.cf.guest_accelerator, []))
    source_image_family  = lookup(local.cf.instance_image, "family", "")
    source_image_project = lookup(local.cf.instance_image, "project", "") # TODO: use normalized project
    source_image         = lookup(local.cf.instance_image, "name", "")

    # subnetwork_project - omit as we use subnewtork self_link
    subnetwork = local.cf.subnetwork_self_link
    spot       = try(local.cf.enable_spot_vm, false)

    termination_action = try(local.cf.spot_instance_config.termination_action, null)
  })

}

module "slurm_cluster" {
  count = var.debug_mode ? 0 : 1

  source = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster?ref=6.1.1"

  project_id         = var.project_id
  slurm_cluster_name = var.name
  region             = var.region
  # TODO: BUCKET
  # TODO: CONTROLLER: CLOUD
  controller_instance_config = local.controller_out

  # TODO: CONTROLLER: HYBRID
  # TODO: LOGIN
  enable_login = false
  nodeset      = local.nodeset_out
  # TODO: nodeset_dyn
  # TODO: nodeset_tpu
  partitions = local.partitions_out
  # TODO: SLURM
  enable_devel = true

  # TODO: move to null resource
  # lifecycle {
  #    precondition {
  #     condition     = length(local.nodeset_out) == length(
  #       flatten([for p in local.partitions_out : p.partition_nodeset])
  #     )
  #     error_message = "Each nodeset must be assigned to an exactly one partition."
  #    }
  # }
  # TODO: preconditions to add:
  # * If var.partition are specified => var.nodeset must be empty
}

locals {
  debug_output = var.debug_mode ? {
    project_id                 = var.project_id
    slurm_cluster_name         = var.name
    region                     = var.region
    controller_instance_config = local.controller_out
    nodeset                    = local.nodeset_out
    partitions                 = local.partitions_out
  } : null
}
