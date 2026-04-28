/**
 * Copyright 2026 Google LLC
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



resource "google_project_service" "dns_api" {
  project            = var.project_id
  service            = "dns.googleapis.com"
  disable_on_destroy = false
}
locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "dns-managed-zone", ghpc_role = "network" })
}
resource "google_dns_managed_zone" "zone" {
  project     = google_project_service.dns_api.project
  name        = var.zone_name
  dns_name    = var.dns_name
  description = var.description
  labels      = local.labels
}
resource "google_dns_record_set" "record" {
  for_each     = { for i, rs in var.recordsets : tostring(i) => rs }
  project      = google_project_service.dns_api.project
  managed_zone = google_dns_managed_zone.zone.name
  name         = each.value.name
  type         = each.value.type
  ttl          = each.value.ttl
  rrdatas      = each.value.rrdatas
}
