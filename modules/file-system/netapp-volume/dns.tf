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
  count = var.dns_config != null ? 1 : 0

  project            = var.project_id
  service            = "dns.googleapis.com"
  disable_on_destroy = false
}

data "google_dns_managed_zone" "mount" {
  count = var.dns_config != null ? 1 : 0

  project = var.project_id
  name    = var.dns_config.managed_zone_name

  depends_on = [google_project_service.dns_api]
}

locals {
  dns_record_name = var.dns_config != null ? coalesce(var.dns_config.record_name, var.volume_name) : null
  mount_fqdn = var.dns_config != null ? (
    endswith(local.dns_record_name, ".") ?
    local.dns_record_name :
    "${local.dns_record_name}.${data.google_dns_managed_zone.mount[0].dns_name}"
  ) : null
  mount_server        = local.mount_fqdn != null ? local.mount_fqdn : local.server_ip
  mount_runner_server = local.mount_fqdn != null ? local.mount_fqdn : join(",", local.server_ips)
}

resource "google_dns_record_set" "volume" {
  count = var.dns_config != null ? 1 : 0

  project      = var.project_id
  managed_zone = var.dns_config.managed_zone_name
  name         = local.mount_fqdn
  type         = "A"
  ttl          = coalesce(var.dns_config.ttl, 60)
  rrdatas      = local.server_ips

  depends_on = [
    google_netapp_volume.netapp_volume,
    data.google_dns_managed_zone.mount,
  ]
}
