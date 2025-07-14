# Copyright 2025 "Google LLC"
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
  replica_zones   = [data.google_compute_zones.zones.names[0], data.google_compute_zones.zones.names[length(data.google_compute_zones.zones.names) - 1]]
  subnetwork_cidr = data.google_compute_subnetwork.subnetwork.ip_cidr_range

}
data "google_compute_zones" "zones" {
  region = var.region
}
data "google_compute_subnetwork" "subnetwork" {
  self_link = var.subnetwork_self_link
}
resource "google_service_account" "slurm_state_storage" {
  project      = var.project_id
  account_id   = "slurm-state-storage-sa"
  display_name = "Cluster slurm-state-storage"
}

resource "google_compute_region_disk" "disk" {
  count         = var.controller_state_disk != null ? 1 : 0
  project       = var.project_id
  name          = "${local.slurm_cluster_name}-slurm-state-regiondisk"
  type          = var.controller_state_disk.type
  region        = var.region
  size          = var.controller_state_disk.size
  replica_zones = local.replica_zones
}

resource "google_compute_firewall" "allow_google" {
  count         = var.controller_state_disk != null ? 1 : 0
  project                 = var.project_id
  name                    = "${local.slurm_cluster_name}-slurm-state-allow-google"
  network                 = data.google_compute_subnetwork.subnetwork.network
  source_ranges           = data.google_netblock_ip_ranges.hcs.cidr_blocks
  target_service_accounts = [google_service_account.slurm_state_storage.email]

  allow {
    protocol = "tcp"
    ports    = ["2049"]
  }
}


resource "google_compute_health_check" "healthcheck" {
  count         = var.controller_state_disk != null ? 1 : 0
  name                = "${local.slurm_cluster_name}-slurm-state-healthcheck"
  project             = var.project_id
  timeout_sec         = 10
  check_interval_sec  = 10
  healthy_threshold   = 1
  unhealthy_threshold = 4

  tcp_health_check {
    port = "2049"
  }
}

resource "google_compute_address" "slurm_state_ip" {
  count         = var.controller_state_disk != null ? 1 : 0
  name         = "${local.slurm_cluster_name}-slurm-state-storage-ip"
  project      = var.project_id
  region       = var.region
  subnetwork   = var.subnetwork_self_link
  address_type = "INTERNAL"
}

resource "google_compute_instance_template" "vm_state_storage" {
  count         = var.controller_state_disk != null ? 1 : 0
  project      = local.controller_project_id
  name_prefix  = "${local.slurm_cluster_name}-slurm-state-storage-template"
  machine_type = var.machine_slurm_state_storage.machine_type
  region       = var.region

  disk {
    source_image = var.machine_slurm_state_storage.disk.source_image
    type         = var.machine_slurm_state_storage.disk.type
    disk_size_gb = var.machine_slurm_state_storage.disk.size
    auto_delete  = true
    boot         = true
  }

  disk {
    source      = google_compute_region_disk.disk[0].id
    auto_delete = false
    boot        = false
  }

  network_interface {
    network_ip = google_compute_address.slurm_state_ip[0].address
    subnetwork = var.subnetwork_self_link
  }

  service_account {
    email  = google_service_account.slurm_state_storage.email
    scopes = try(var.service_account.scopes, ["https://www.googleapis.com/auth/cloud-platform"])
  }

  metadata_startup_script = <<EOF
    hostnamectl set-hostname ${local.slurm_cluster_name}-slurm-state-storage

    dnf install -y nfs-utils

    lsblk /dev/sdb1 || sgdisk -n 1: /dev/sdb && partprobe
    lsblk -f /dev/sdb1 | grep xfs || mkfs.xfs /dev/sdb1
    mkdir -p /var/spool/slurm
    mount -t xfs /dev/sdb1 /var/spool/slurm

    mkdir -p /opt/apps
    mkdir -p /etc/munge

    # Add NFS exports
    echo -e '/var/spool/slurm\t${local.subnetwork_cidr}(rw,sync,no_root_squash)' | tee -a /etc/exports
    echo -e '/home\t${local.subnetwork_cidr}(rw,sync,no_root_squash)' | tee -a /etc/exports
    echo -e '/opt/apps\t${local.subnetwork_cidr}(rw,sync,no_root_squash)' | tee -a /etc/exports
    echo -e '/etc/munge\t${local.subnetwork_cidr}(rw,sync,no_root_squash)' | tee -a /etc/exports

    # Apply export rules
    exportfs -a

    # Enable and restart NFS server
    systemctl enable --now nfs-server
    systemctl restart nfs-server
  EOF
  lifecycle {
    create_before_destroy = true
  }
}

resource "google_compute_region_instance_group_manager" "mig" {
  count         = var.controller_state_disk != null ? 1 : 0
  project = var.project_id
  name    = "${local.slurm_cluster_name}-slurm-state-mig"

  base_instance_name = "slurm-state-storage"
  region             = var.region
  target_size        = 1

  version {
    instance_template = google_compute_instance_template.vm_state_storage[0].id
  }

  wait_for_instances = false

  auto_healing_policies {
    health_check      = google_compute_health_check.healthcheck[0].id
    initial_delay_sec = 300
  }

  update_policy {
    type                         = "PROACTIVE"
    instance_redistribution_type = "PROACTIVE"
    minimal_action               = "REPLACE"
    replacement_method           = "RECREATE"
    max_surge_fixed              = 0
    max_unavailable_fixed        = 4
  }

  distribution_policy_zones = local.replica_zones
}
