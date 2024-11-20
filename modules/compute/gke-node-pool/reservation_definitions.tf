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

data "google_compute_reservation" "specific_reservations" {
  for_each = (
    local.input_specific_reservations_count == 0 ?
    {} :
    {
      for pair in flatten([
        for zone in try(var.zones, []) : [
          for reservation in try(var.reservation_affinity.specific_reservations, []) : {
            key : "${coalesce(reservation.project, var.project_id)}/${zone}/${reservation.name}"
            zone : zone
            reservation_name : reservation.name
            project : reservation.project == null ? var.project_id : reservation.project
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
  reservation_resource_api_label    = "compute.googleapis.com/reservation-name"
  input_specific_reservations_count = try(length(var.reservation_affinity.specific_reservations), 0)

  # Filter specific reservations
  verified_specific_reservations = [for k, v in data.google_compute_reservation.specific_reservations : v if(v.specific_reservation != null && v.specific_reservation_required == true)]

  # Build two maps to be used to compare the VM properties between reservations and the node pool
  reservation_vm_properties = [for r in local.verified_specific_reservations : {
    "machine_type" : try(r.specific_reservation[0].instance_properties[0].machine_type, "")
    "guest_accelerators" : { for acc in try(r.specific_reservation[0].instance_properties[0].guest_accelerators, []) : acc.accelerator_type => acc.accelerator_count },
    "local_ssds" : {
      "NVME" : length([for d in try(r.specific_reservation[0].instance_properties[0].local_ssds, []) : d if d.interface == "NVME"])
      "SCSI" : length([for d in try(r.specific_reservation[0].instance_properties[0].local_ssds, []) : d if d.interface == "SCSI"])
    }
  }]
  nodepool_vm_properties = {
    "machine_type" : var.machine_type
    "guest_accelerators" : { for acc in try(local.guest_accelerator, []) : coalesce(acc.type, try(local.generated_guest_accelerator[0].type, "")) => coalesce(acc.count, try(local.generated_guest_accelerator[0].count, 0)) },
    "local_ssds" : {
      "NVME" : coalesce(local.local_ssd_config.local_ssd_count_nvme_block, 0),
      "SCSI" : coalesce(local.local_ssd_config.local_ssd_count_ephemeral_storage, 0)
    }
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
    "local_ssds" : <<-EOT
    The reservation has ${jsonencode(try(local.reservation_vm_properties[0].local_ssds, {}))} local SSDs and the node pool has ${jsonencode(try(local.nodepool_vm_properties.local_ssds, {}))}. Check the relevant node pool settings: {local_ssd_count_ephemeral_storage, local_ssd_count_nvme_block}. When unspecified, for the machine_type=${var.machine_type} the defaults are: {local_ssd_count_ephemeral_storage=${coalesce(local.generated_local_ssd_config.local_ssd_count_ephemeral_storage, 0)}, local_ssd_count_nvme_block=${coalesce(local.generated_local_ssd_config.local_ssd_count_nvme_block, 0)}}.
    EOT
  }
}
