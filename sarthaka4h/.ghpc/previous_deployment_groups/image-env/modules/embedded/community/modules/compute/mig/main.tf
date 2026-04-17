# Copyright 2024 Google LLC
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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "mig", ghpc_role = "compute" })
}

locals {
  sanitized_deploy_name = try(replace(lower(var.deployment_name), "/[^a-z0-9]/", ""), null)
  sanitized_module_id   = try(replace(lower(var.ghpc_module_id), "/[^a-z0-9]/", ""), null)
  synth_mig_name        = try("${local.sanitized_deploy_name}-${local.sanitized_module_id}", null)

  mig_name           = var.name == null ? local.synth_mig_name : var.name
  base_instance_name = var.base_instance_name == null ? local.mig_name : var.base_instance_name
}

resource "google_compute_instance_group_manager" "mig" {
  # REQUIRED
  name               = local.mig_name
  base_instance_name = local.base_instance_name
  zone               = var.zone

  dynamic "version" {
    for_each = var.versions
    content {
      name              = version.value.name
      instance_template = version.value.instance_template
      dynamic "target_size" {
        for_each = version.value.target_size != null ? [version.value.target_size] : []
        content {
          fixed   = target_size.value.fixed
          percent = target_size.value.percent
        }
      }
    }
  }

  # OPTIONAL
  project            = var.project_id
  target_size        = var.target_size
  wait_for_instances = var.wait_for_instances

  all_instances_config {
    # TODO: validate that template metadata not getting wiped out
    # TODO: validate that template labels not getting wiped out
    labels = local.labels
  }

  # OMITTED:
  # * description
  # * named_port 
  # * list_managed_instances_results 
  # * target_pools - specific for Load Balancers usage
  # * wait_for_instances_status
  # * auto_healing_policies
  # * stateful_disk
  # * stateful_internal_ip
  # * update_policy
  # * params 


  lifecycle {
    precondition {
      condition     = local.mig_name != null
      error_message = "Could not come up with a name for the MIG, specify `var.name`"
    }

    precondition {
      condition     = local.base_instance_name != null
      error_message = "Could not come up with a base_instance_name, specify `var.base_instance_name`"
    }
  }
}
