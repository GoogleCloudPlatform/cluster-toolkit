# Copyright 2026 "Google LLC"
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
  labels = merge(var.labels, { ghpc_module = "managed-certificate", ghpc_role = "network" })
}

locals {
  # A wildcard is authorised by its parent domain's record, so *.example.com
  # and example.com share one authorisation.
  authorized_domains = toset([for domain in var.domains : trimprefix(domain, "*.")])
}

# DNS authorisation rather than load balancer authorisation: it is the only kind
# a wildcard can use, and it lets the certificate be issued before any load
# balancer serves the domain.
resource "google_certificate_manager_dns_authorization" "domains" {
  for_each = local.authorized_domains

  project = var.project_id
  name    = "${var.name}-${substr(sha1(each.key), 0, 8)}"
  domain  = each.key
  labels  = local.labels
}

resource "google_certificate_manager_certificate" "certificate" {
  project = var.project_id
  name    = var.name
  labels  = local.labels

  managed {
    domains            = var.domains
    dns_authorizations = [for authorization in google_certificate_manager_dns_authorization.domains : authorization.id]
  }
}

resource "google_certificate_manager_certificate_map" "map" {
  project = var.project_id
  name    = var.name
  labels  = local.labels
}

# One entry per domain, matched by hostname. Identity-Aware Proxy refuses
# requests served through a PRIMARY entry, the map's catch-all, with a
# hostname and certificate mismatch.
resource "google_certificate_manager_certificate_map_entry" "domains" {
  for_each = toset(var.domains)

  project      = var.project_id
  name         = "${var.name}-${substr(sha1(each.key), 0, 8)}"
  map          = google_certificate_manager_certificate_map.map.name
  hostname     = each.key
  certificates = [google_certificate_manager_certificate.certificate.id]
  labels       = local.labels
}
