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
}
