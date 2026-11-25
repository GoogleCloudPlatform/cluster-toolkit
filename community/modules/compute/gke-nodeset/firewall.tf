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

# GKE nodes mount the Slurm key directory from the controller over NFS (see
# persistent_volumes.tf). The controller module denies NFS to the controller
# from all sources except instances tagged with the cluster name, and GKE nodes
# do not carry that tag, so allow NFS from this nodeset's GKE subnetwork.
# This rule uses the same priority as the controller's allow rules (998), which
# takes precedence over its deny rule (999).
locals {
  nfs_firewall_name = trimsuffix(substr("${var.slurm_cluster_name}-${var.nodeset_name}-nfs", 0, 63), "-")
  nfs_ports         = ["111", "2049", "20048"]

  # The controller is tagged "<cluster>" and "<cluster>-controller", where
  # <cluster> is the controller's own slurm_cluster_name. Read the tag from the
  # instance rather than rebuilding it, in case the names differ.
  controller_tags = tolist(try(var.slurm_controller_instance.tags, []))

  controller_nfs_target_tag = try(
    [for t in local.controller_tags : t if endswith(t, "-controller") && contains(local.controller_tags, trimsuffix(t, "-controller"))][0],
    "${var.slurm_cluster_name}-controller",
  )
}

resource "google_compute_firewall" "controller_nfs_allow_gke" {
  count       = var.create_nfs_firewall_rule ? 1 : 0
  name        = local.nfs_firewall_name
  description = "Allow NFS traffic to the Slurm controller from GKE nodeset ${var.nodeset_name}"
  project     = var.subnetwork.project
  network     = var.slurm_controller_instance.network_interface[0].network

  direction     = "INGRESS"
  priority      = 998
  source_ranges = [var.subnetwork.ip_cidr_range]
  target_tags   = [local.controller_nfs_target_tag]

  allow {
    protocol = "tcp"
    ports    = local.nfs_ports
  }
  allow {
    protocol = "udp"
    ports    = local.nfs_ports
  }
}
