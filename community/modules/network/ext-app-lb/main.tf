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

# The front end of a global external Application Load Balancer: address,
# certificate, URL map and forwarding rules. Its backend services are created
# elsewhere, such as by iap-backend-service, so one front end can serve
# backends owned by several deployments.

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "ext-app-lb", ghpc_role = "network" })
}

locals {
  ssl = (
    var.certificate_map != null
    || length(var.ssl_certificates) > 0
    || length(var.managed_ssl_certificate_domains) > 0
  )

  # One entry per path. A host's "/*" entry catches whatever its other paths
  # do not.
  url_map_input = flatten([
    for route in var.routes : [
      for path in route.paths : {
        host            = route.host
        path            = path
        backend_service = route.backend_service
      }
    ]
  ])
}

# Google's default policy, which applies when a proxy names none, still allows
# TLS 1.0.
resource "google_compute_ssl_policy" "policy" {
  count           = local.ssl && var.ssl_policy == null ? 1 : 0
  project         = var.project_id
  name            = "${var.name}-tls"
  profile         = "MODERN"
  min_tls_version = var.min_tls_version
}

module "frontend" {
  source  = "GoogleCloudPlatform/lb-http/google//modules/frontend"
  version = "~> 14.3"

  project_id            = var.project_id
  name                  = var.name
  load_balancing_scheme = "EXTERNAL_MANAGED"
  labels                = local.labels

  create_address = var.address == null
  address        = var.address

  url_map_input = local.url_map_input

  ssl                             = local.ssl
  certificate_map                 = var.certificate_map
  ssl_certificates                = var.ssl_certificates
  managed_ssl_certificate_domains = var.managed_ssl_certificate_domains
  # Lets the managed certificate's domains change: a certificate cannot be
  # replaced in place while a proxy uses it.
  random_certificate_suffix = length(var.managed_ssl_certificate_domains) > 0
  ssl_policy                = var.ssl_policy != null ? var.ssl_policy : one(google_compute_ssl_policy.policy[*].self_link)

  # Without a certificate there is nothing to redirect to.
  https_redirect = local.ssl && var.https_redirect
}
