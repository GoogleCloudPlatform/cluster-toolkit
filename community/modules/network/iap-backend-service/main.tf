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

# A backend service behind Identity-Aware Proxy, for a global external
# Application Load Balancer whose URL map routes a hostname here. The load
# balancer's front end - address, certificate and URL map - is not created
# here, so one front end can serve the desktops of several deployments.

# A network endpoint group rather than an instance group, so the backend
# service does not depend on the instances. The hosts need the backend
# service's name to verify IAP assertions, and an instance group would need
# the hosts first.
resource "google_compute_network_endpoint_group" "desktops" {
  project               = var.project_id
  name                  = var.name
  zone                  = var.zone
  network               = var.network_self_link
  subnetwork            = var.subnetwork_self_link
  network_endpoint_type = "GCE_VM_IP_PORT"
  default_port          = var.port
}

resource "google_compute_network_endpoint" "desktops" {
  count                  = length(var.endpoints)
  project                = var.project_id
  zone                   = var.zone
  network_endpoint_group = google_compute_network_endpoint_group.desktops.name
  instance               = var.endpoints[count.index].instance
  ip_address             = var.endpoints[count.index].ip_address
  port                   = var.port
}

module "backend" {
  source  = "GoogleCloudPlatform/lb-http/google//modules/backend"
  version = "~> 14.3"

  project_id            = var.project_id
  name                  = var.name
  load_balancing_scheme = "EXTERNAL_MANAGED"
  protocol              = "HTTP"

  # A desktop is a websocket held open for hours. The default of 30 seconds
  # would close it mid-session.
  timeout_sec = var.timeout_sec
  # A user's session lives on one host, so send them back to it.
  session_affinity = "GENERATED_COOKIE"

  groups = [{
    group                 = google_compute_network_endpoint_group.desktops.id
    balancing_mode        = "RATE"
    max_rate_per_endpoint = 1000
  }]

  health_check = {
    request_path = var.health_check_path
    port         = var.port
    protocol     = "HTTP"
  }

  # Its firewall rules come in a pair, the second opening 80, 443 and 8080 to
  # an internal proxy range this load balancer does not use. Admit Google's
  # front ends to port with modules/network/firewall-rules instead.
  firewall_networks = []

  # IAP decides who reaches the hosts; OS Login, asked by the broker, still
  # decides who gets a desktop on them. Grant both.
  iap_config = {
    enable               = true
    oauth2_client_id     = var.oauth2_client_id
    oauth2_client_secret = var.oauth2_client_secret
    iap_members          = var.iap_members
  }
}
