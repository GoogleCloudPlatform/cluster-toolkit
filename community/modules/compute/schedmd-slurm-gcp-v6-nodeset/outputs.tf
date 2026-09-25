# Copyright 2026 Google LLC
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

output "nodeset" {
  description = "Details of the nodeset. Typically used as input to `schedmd-slurm-gcp-v6-partition`."
  value       = local.nodeset

  precondition {
    condition = alltrue([
      for mt in setunion([var.machine_type], local.flex_machine_types) : !contains([
        "c2:hyperdisk-balanced",
        "c2:hyperdisk-extreme",
        "c2:hyperdisk-throughput",
        "c3:pd-standard",
        "c3d:pd-standard",
        "c4:pd-standard",
        "c4:pd-balanced",
        "c4:pd-ssd",
        "c4a:pd-standard",
        "c4a:pd-balanced",
        "c4a:pd-ssd",
        "c4d:pd-standard",
        "c4d:pd-balanced",
        "c4d:pd-ssd",
        "h3:pd-standard",
        "h3:pd-ssd",
        "h4d:pd-standard",
        "h4d:pd-balanced",
        "h4d:pd-ssd",
        "n4:pd-standard",
        "n4:pd-balanced",
        "n4:pd-ssd",
        "n4a:pd-standard",
        "n4a:pd-balanced",
        "n4a:pd-ssd",
        "n4d:pd-standard",
        "n4d:pd-balanced",
        "n4d:pd-ssd",
      ], "${split("-", mt)[0]}:${var.disk_type}")
    ])
    error_message = "A disk_type=${var.disk_type} cannot be used with machine_type=${var.machine_type} or one of its instance_flexibility_policy fallback machine types."
  }

  precondition {
    condition     = var.reservation_name == "" || length(var.zones) == 0
    error_message = <<-EOD
      If a reservation is specified, `var.zones` should be empty.
    EOD
  }

  precondition {
    condition     = var.accelerator_topology == null || var.accelerator_topology == "" || var.enable_placement
    error_message = "accelerator_topology requires enable_placement to be set to true."
  }

  precondition {
    condition     = !(var.provisioning_engine == "MIG" && !var.dws_flex.enabled && var.enable_placement && (var.accelerator_topology == null || var.accelerator_topology == ""))
    error_message = "MIG provisioning engine only supports enable_placement when accelerator_topology is specified (e.g. '1x72')."
  }

  precondition {
    condition     = var.accelerator_topology == null || var.accelerator_topology == "" || can(regex("^[1-9][0-9]*[xX][1-9][0-9]*$", trimspace(var.accelerator_topology)))
    error_message = "accelerator_topology must be formatted as '<dim1>x<dim2>' with positive integers (e.g. '1x72')."
  }

  precondition {
    condition     = var.accelerator_topology == null || var.accelerator_topology == "" || length(local.guest_accelerator) > 0 || can(regex("^(a[2-4]x?|g2)", var.machine_type))
    error_message = "accelerator_topology can only be configured on machine types with attached GPUs or accelerators."
  }

  # GCE accepts acceleratorTopology in a HIGH_THROUGHPUT workload policy only on A4X / A4X Max;
  # Bulk Insert binds it via runtime placement policies instead, hence the MIG-only gate.
  precondition {
    condition     = !(var.provisioning_engine == "MIG" && !var.dws_flex.enabled && var.accelerator_topology != null && var.accelerator_topology != "") || can(regex("^a4x-", var.machine_type))
    error_message = "accelerator_topology with provisioning_engine = 'MIG' requires a machine type that supports HIGH_THROUGHPUT workload policies with an accelerator topology (A4X, A4X Max). Use provisioning_engine = 'BULK_INSERT' for other machine types."
  }

  precondition {
    condition     = (var.accelerator_topology == null || var.accelerator_topology == "") || try(tonumber(split("x", lower(trimspace(var.accelerator_topology)))[1]) % local.gpu_count == 0, false)
    error_message = "The second dimension (<dim2>) of accelerator_topology must be divisible by the number of GPUs per machine."
  }

  precondition {
    condition = (var.accelerator_topology == null || var.accelerator_topology == "" || var.provisioning_engine != "MIG" || var.dws_flex.enabled) || (
      var.node_count_static > 0 && try(
        var.node_count_static % (
          (tonumber(split("x", lower(trimspace(var.accelerator_topology)))[0]) * tonumber(split("x", lower(trimspace(var.accelerator_topology)))[1])) / local.gpu_count
        ) == 0,
        false
      )
    )
    error_message = "When accelerator_topology is specified with provisioning_engine = 'MIG', node_count_static must be greater than 0 and an integer multiple of the slice size ((dim1 * dim2) / gpus_per_machine)."
  }

  precondition {
    condition     = var.placement_max_distance == null || var.enable_placement
    error_message = "placement_max_distance requires enable_placement to be set to true."
  }

  precondition {
    condition     = !(startswith(var.machine_type, "a3-") && var.placement_max_distance == 1)
    error_message = "A3 machines do not support a placement_max_distance of 1."
  }

  precondition {
    condition     = var.reservation_name == "" || !var.dws_flex.enabled
    error_message = "Cannot use reservations with DWS Flex."
  }

  precondition {
    condition     = length(var.zones) == 0 || !var.dws_flex.enabled
    error_message = <<-EOD
      If a DWS Flex is enabled, `var.zones` should be empty.
    EOD
  }

  precondition {
    condition     = var.on_host_maintenance == "TERMINATE" || !var.dws_flex.enabled
    error_message = "If DWS Flex is used, `on_host_maintenance` should be set to 'TERMINATE'"
  }

  precondition {
    condition     = !var.enable_spot_vm || !var.dws_flex.enabled
    error_message = "Cannot use both Flex-Start and Spot VMs for provisioning."
  }

  precondition {
    condition     = var.reservation_name == "" || var.future_reservation == ""
    error_message = "Cannot use reservations and future reservations in the same nodeset"
  }

  precondition {
    condition     = !var.enable_placement || var.future_reservation == ""
    error_message = "Cannot use `enable_placement` with future reservations."
  }

  precondition {
    condition     = var.future_reservation == "" || length(var.zones) == 0
    error_message = <<-EOD
      If a future reservation is specified, `var.zones` should be empty.
    EOD
  }

  precondition {
    condition     = var.future_reservation == "" || local.fr_zone == var.zone
    error_message = <<-EOD
      The zone of the deployment must match that of the future reservation
    EOD
  }

  precondition {
    condition     = var.node_count_dynamic_max > 0 || var.node_count_static > 0
    error_message = <<-EOD
      This nodeset contains zero nodes, there should be at least one static or dynamic node
    EOD
  }

  precondition {
    condition     = !(var.dws_flex.enabled && !var.dws_flex.use_bulk_insert && var.provisioning_engine == "BULK_INSERT")
    error_message = "DWS Flex-Start strictly requires MIGs. Cannot force provisioning_engine = 'BULK_INSERT'."
  }

  precondition {
    condition = !(
      var.node_count_dynamic_max > 0 && var.provisioning_engine == "MIG" && !var.dws_flex.enabled
    )
    error_message = "Dynamic compute NodeSets with provisioning_engine = 'MIG' are currently not supported. Please explicitly set node_count_dynamic_max = 0."
  }

  precondition {
    condition = local.zone_target_shape == "ANY_SINGLE_ZONE" || !(
      local.mig_provisioned &&
      (var.enable_placement || (var.accelerator_topology != null && var.accelerator_topology != ""))
    )
    error_message = "zone_target_shape must be 'ANY_SINGLE_ZONE' on MIG NodeSets using enable_placement or accelerator_topology; those policies are zonal."
  }

  precondition {
    condition     = !(var.dws_flex.enabled && !var.dws_flex.use_bulk_insert) || contains(["ANY_SINGLE_ZONE", "ANY"], local.zone_target_shape)
    error_message = "DWS Flex (FLEX_START) Regional MIGs only support zone_target_shape of 'ANY_SINGLE_ZONE' or 'ANY'; 'BALANCED' is rejected by Compute Engine."
  }

  precondition {
    condition     = !local.has_flex_policy || (var.provisioning_engine == "MIG" && !var.dws_flex.enabled)
    error_message = "instance_flexibility_policy requires provisioning_engine = 'MIG' and is not supported with DWS Flex."
  }

  precondition {
    condition     = !local.has_flex_policy || var.accelerator_topology == null || var.accelerator_topology == ""
    error_message = "instance_flexibility_policy cannot be combined with accelerator_topology."
  }

  precondition {
    condition     = !local.has_flex_policy || (var.reservation_name == "" && var.future_reservation == "")
    error_message = "instance_flexibility_policy cannot be combined with a reservation or future reservation; a specific reservation pins a single machine type."
  }

  precondition {
    condition = !local.has_flex_policy || alltrue([
      for mt in setunion([var.machine_type], local.flex_machine_types) :
      !can(regex("^(a3-ultragpu|a4|a4x)-", mt))
    ])
    error_message = "instance_flexibility_policy is not supported on a3-ultragpu, a4, or a4x machine types."
  }

  precondition {
    condition = !local.has_flex_policy || (
      alltrue([
        for mt in local.flex_machine_types :
        local.inferred_gpu_signature[mt] == local.inferred_gpu_signature[var.machine_type]
        ]) && (
        length(var.guest_accelerator) == 0 || alltrue([
          for mt in local.flex_machine_types : startswith(mt, "n1-")
        ])
      )
    )
    error_message = "Every instance_flexibility_policy machine type must have the same attached GPU model and count as machine_type=${var.machine_type} (and must be an n1-* shape when guest_accelerator is explicitly set)."
  }

  precondition {
    condition = !local.has_flex_policy || alltrue([
      for mt in local.flex_machine_types :
      can(regex("^(t2a|c4a|n4a)-", mt)) == can(regex("^(t2a|c4a|n4a)-", var.machine_type))
    ])
    error_message = "All instance_flexibility_policy machine types must have the same CPU architecture (x86_64 or Arm64) as machine_type=${var.machine_type}."
  }

  precondition {
    condition = !local.has_flex_policy || (
      length(setunion([var.machine_type], local.flex_machine_types)) <= 10 &&
      length(distinct([for s in local.instance_flexibility_policy.instance_selections : s.name])) == length(local.instance_flexibility_policy.instance_selections)
    )
    error_message = "instance_flexibility_policy may reference at most 10 distinct machine types in total (including machine_type=${var.machine_type}) and all selection names must be unique."
  }
}
