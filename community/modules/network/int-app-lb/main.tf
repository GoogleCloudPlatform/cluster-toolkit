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

locals {
  is_https = length(var.ssl_certificates) > 0
  # We need a default service for the URL map. We just pick the first endpoint alphabetically.
  default_endpoint_name = keys(var.endpoints)[0]
}

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "int-app-lb", ghpc_role = "network" })
}

resource "google_compute_region_target_http_proxy" "default" {
  count   = local.is_https ? 0 : 1
  project = var.project_id
  region  = var.region
  name    = "${var.lb_name}-http-proxy"
  url_map = google_compute_region_url_map.default.id
}

resource "google_compute_region_target_https_proxy" "default" {
  count            = local.is_https ? 1 : 0
  project          = var.project_id
  region           = var.region
  name             = "${var.lb_name}-https-proxy"
  url_map          = google_compute_region_url_map.default.id
  ssl_certificates = var.ssl_certificates
}

resource "google_compute_address" "default" {
  name         = "${var.lb_name}-ip"
  project      = var.project_id
  region       = var.region
  subnetwork   = var.subnetwork_self_link
  address_type = "INTERNAL"
  address      = var.ip_address
}

resource "google_compute_forwarding_rule" "default" {
  project               = var.project_id
  region                = var.region
  name                  = "${var.lb_name}-forwarding-rule"
  network               = var.network_self_link
  subnetwork            = var.subnetwork_self_link
  ip_address            = google_compute_address.default.address
  load_balancing_scheme = "INTERNAL_MANAGED"
  port_range            = local.is_https ? "443" : "80"
  target                = local.is_https ? google_compute_region_target_https_proxy.default[0].id : google_compute_region_target_http_proxy.default[0].id
  labels                = local.labels
}

resource "google_compute_region_backend_service" "default" {
  for_each = var.endpoints
  project  = var.project_id
  region   = var.region
  name     = "${var.lb_name}-backend-${each.key}"

  load_balancing_scheme = "INTERNAL_MANAGED"
  protocol              = var.protocol
  port_name             = each.key

  timeout_sec                     = var.timeout_sec
  connection_draining_timeout_sec = var.connection_draining_timeout_sec

  health_checks = [google_compute_region_health_check.default[each.key].id]

  dynamic "backend" {
    for_each = var.backends
    content {
      group           = backend.value
      balancing_mode  = "UTILIZATION"
      capacity_scaler = 1.0
    }
  }

  dynamic "log_config" {
    for_each = var.log_config != null ? [var.log_config] : []
    content {
      enable      = log_config.value.enable
      sample_rate = log_config.value.sample_rate
    }
  }

  dynamic "iap" {
    for_each = var.iap_config != null ? [var.iap_config] : []
    content {
      enabled              = true
      oauth2_client_id     = iap.value.oauth2_client_id
      oauth2_client_secret = iap.value.oauth2_client_secret
    }
  }
}

resource "google_compute_region_health_check" "default" {
  for_each = var.endpoints
  project  = var.project_id
  region   = var.region
  name     = "${var.lb_name}-hc-${each.key}"

  check_interval_sec  = var.health_check_interval_sec
  timeout_sec         = var.health_check_timeout_sec
  healthy_threshold   = var.healthy_threshold
  unhealthy_threshold = var.unhealthy_threshold

  dynamic "http_health_check" {
    for_each = each.value.health_check_type == "HTTP" ? [1] : []
    content {
      port         = each.value.port
      request_path = each.value.health_check_request_path
    }
  }

  dynamic "tcp_health_check" {
    for_each = each.value.health_check_type == "TCP" ? [1] : []
    content {
      port = each.value.port
    }
  }
}

resource "google_compute_region_url_map" "default" {
  project         = var.project_id
  region          = var.region
  name            = "${var.lb_name}-url-map"
  default_service = google_compute_region_backend_service.default[local.default_endpoint_name].id

  host_rule {
    hosts        = ["*"]
    path_matcher = "allpaths"
  }

  path_matcher {
    name            = "allpaths"
    default_service = google_compute_region_backend_service.default[local.default_endpoint_name].id

    dynamic "path_rule" {
      for_each = flatten([
        for name, config in var.endpoints : [
          for path in config.paths : {
            path    = path
            service = google_compute_region_backend_service.default[name].id
          }
        ]
      ])
      content {
        paths   = [path_rule.value.path]
        service = path_rule.value.service
      }
    }
  }
}
