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

# Limits which hosts can mount NFS exports from the controller (/home,
# /opt/apps, the Slurm key directory, ...). Exports are limited to the primary
# subnetwork ranges of the controller, login nodes and nodesets, plus
# var.extra_nfs_client_cidrs, and controller NFS ports are firewalled to cluster
# instances.

locals {
  # Index 0 is always the controller subnetwork. The list is not de-duplicated
  # so that its length is known at plan time even when subnetworks are created
  # in the same apply.
  nfs_client_subnetworks = var.enable_hybrid ? [] : concat(
    [var.subnetwork_self_link],
    [for x in var.login_nodes : x.subnetwork],
    [for ns in var.nodeset : ns.subnetwork_self_link],
    # coalesce keeps the list length known at plan time
    [for ns in var.nodeset_dyn : coalesce(ns.subnetwork_self_link, var.subnetwork_self_link)],
  )

  nfs_export_clients = distinct(concat(
    data.google_compute_subnetwork.nfs_clients[*].ip_cidr_range,
    var.extra_nfs_client_cidrs,
  ))

  nfs_ports           = ["111", "2049", "20048"]
  create_nfs_firewall = var.create_nfs_firewall_rules && !var.enable_hybrid
  controller_net_proj = length(data.google_compute_subnetwork.nfs_clients) > 0 ? data.google_compute_subnetwork.nfs_clients[0].project : null
  controller_net_link = length(data.google_compute_subnetwork.nfs_clients) > 0 ? data.google_compute_subnetwork.nfs_clients[0].network : null
  controller_net_tag  = "${local.slurm_cluster_name}-controller"
}

data "google_compute_subnetwork" "nfs_clients" {
  count     = length(local.nfs_client_subnetworks)
  self_link = local.nfs_client_subnetworks[count.index]
}

# The default VPC "allow internal traffic" rule (priority 1000) admits all
# internal TCP/UDP traffic, so restricting NFS needs a deny rule with a higher
# priority, plus an even higher-priority allow rule for cluster instances.
resource "google_compute_firewall" "controller_nfs_deny" {
  count       = local.create_nfs_firewall ? 1 : 0
  name        = "${local.slurm_cluster_name}-controller-nfs-deny"
  description = "Deny NFS traffic to the Slurm controller from hosts outside the cluster"
  project     = local.controller_net_proj
  network     = local.controller_net_link

  direction     = "INGRESS"
  priority      = 999
  source_ranges = ["0.0.0.0/0"]
  target_tags   = [local.controller_net_tag]

  deny {
    protocol = "tcp"
    ports    = local.nfs_ports
  }
  deny {
    protocol = "udp"
    ports    = local.nfs_ports
  }

  depends_on = [
    google_compute_firewall.controller_nfs_allow,
    google_compute_firewall.controller_nfs_allow_extra,
  ]
}

# Allow NFS from cluster instances (all carry the cluster name tag).
resource "google_compute_firewall" "controller_nfs_allow" {
  count       = local.create_nfs_firewall ? 1 : 0
  name        = "${local.slurm_cluster_name}-controller-nfs-allow"
  description = "Allow NFS traffic to the Slurm controller from cluster instances"
  project     = local.controller_net_proj
  network     = local.controller_net_link

  direction   = "INGRESS"
  priority    = 998
  source_tags = [local.slurm_cluster_name]
  target_tags = [local.controller_net_tag]

  allow {
    protocol = "tcp"
    ports    = local.nfs_ports
  }
  allow {
    protocol = "udp"
    ports    = local.nfs_ports
  }
}

# Allow NFS from var.extra_nfs_client_cidrs. This is a separate rule that only
# exists when ranges are set: source_ranges is optional and computed in the
# provider, so setting it to null on a shared rule would not remove old ranges.
resource "google_compute_firewall" "controller_nfs_allow_extra" {
  count       = local.create_nfs_firewall && length(var.extra_nfs_client_cidrs) > 0 ? 1 : 0
  name        = "${local.slurm_cluster_name}-controller-nfs-allow-extra"
  description = "Allow NFS traffic to the Slurm controller from extra_nfs_client_cidrs"
  project     = local.controller_net_proj
  network     = local.controller_net_link

  direction     = "INGRESS"
  priority      = 998
  source_ranges = var.extra_nfs_client_cidrs
  target_tags   = [local.controller_net_tag]

  allow {
    protocol = "tcp"
    ports    = local.nfs_ports
  }
  allow {
    protocol = "udp"
    ports    = local.nfs_ports
  }
}
