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

# Split the input into three different lists where the details of a given reservation are at the same index across these lists. 
locals {
  # Specific block of an extended reservation can be targeted with exr-one/reservationBlocks/exr-one-block-1
  # Data source needs to be queried with the reservation name only. So, we extract the reservation name
  input_reservation_names    = [for r in try(var.reservation_affinity.specific_reservations, []) : split("/", r.name)[0]]
  input_reservation_projects = [for r in try(var.reservation_affinity.specific_reservations, []) : coalesce(r.project, var.project_id)]
  # We, also, remember the suffix "/reservationBlocks/exr-one-block-1" for use elsewhere afterwards
  input_reservation_suffixes = [for r in try(var.reservation_affinity.specific_reservations, []) : substr(r.name, length(split("/", r.name)[0]), -1)]
}

data "google_compute_reservation" "specific_reservations" {
  for_each = (
    local.input_specific_reservations_count == 0 ?
    {} :
    {
      for pair in flatten([
        for zone in try(var.zones, []) : [
          for i, reservation_name in try(local.input_reservation_names, []) : {
            key : "${local.input_reservation_projects[i]}/${zone}/${reservation_name}"
            zone : zone
            reservation_name : reservation_name
            project : local.input_reservation_projects[i]
          }
        ]
      ]) :
      pair.key => pair
    }
  )
  name    = each.value.reservation_name
  zone    = each.value.zone
  project = each.value.project
}

locals {
  generated_guest_accelerator       = module.gpu.machine_type_guest_accelerator
  reservation_resource_api_label    = "compute.googleapis.com/reservation-name"
  input_specific_reservations_count = try(length(var.reservation_affinity.specific_reservations), 0)

  # Filter specific reservations
  verified_specific_reservations = [for k, v in data.google_compute_reservation.specific_reservations : v if(v.specific_reservation != null && v.specific_reservation_required == true)]

  # Build two maps to be used to compare the VM properties between reservations and the node pool
  reservation_vm_properties = [for r in local.verified_specific_reservations : {
    "machine_type" : try(r.specific_reservation[0].instance_properties[0].machine_type, "")
    "guest_accelerators" : { for acc in try(r.specific_reservation[0].instance_properties[0].guest_accelerators, []) : acc.accelerator_type => acc.accelerator_count }
  }]
  nodepool_vm_properties = {
    "machine_type" : var.machine_type
    "guest_accelerators" : { for acc in try(local.guest_accelerator, []) : coalesce(acc.type, try(local.generated_guest_accelerator[0].type, "")) => coalesce(acc.count, try(local.generated_guest_accelerator[0].count, 0)) }
  }

  # Compare two maps by counting the keys that mismatch.
  # Know that in map comparison the order of keys does not matter. That is {NVME: x, SCSI: y} and {SCSI: y, NVME: x} are equal
  # As of this writing, there is only one reservation supported by the Node Pool API. So, directly accessing it from the list
  specific_reservation_requirement_violations = length(local.reservation_vm_properties) == 0 ? [] : [for k, v in local.nodepool_vm_properties : k if v != local.reservation_vm_properties[0][k]]

  specific_reservation_requirement_violation_messages = {
    "machine_type" : <<-EOT
    The reservation has "${try(local.reservation_vm_properties[0].machine_type, "")}" machine type and the node pool has "${local.nodepool_vm_properties.machine_type}". Check the relevant node pool setting: "machine_type"
    EOT
    "guest_accelerators" : <<-EOT
    The reservation has ${jsonencode(try(local.reservation_vm_properties[0].guest_accelerators, {}))} accelerators and the node pool has ${jsonencode(try(local.nodepool_vm_properties.guest_accelerators, {}))}. Check the relevant node pool setting: "guest_accelerator". When unspecified, for the machine_type=${var.machine_type}, the default is guest_accelerator=${jsonencode(try(local.generated_guest_accelerator, [{}]))}.
    EOT
  }
}
