/**
 * Copyright 2021 Google LLC
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

resource "google_compute_disk" "default" {
  name  = "${var.deployment_name}-disk"
  image = var.image_family
  size  = var.disk_size
  type  = var.type
  zone  = var.zone
}

resource "google_compute_instance" "compute_instance" {
  name                    = "${var.deployment_name}-nfs-instance"
  zone                    = var.zone
  machine_type            = var.machine_type
  metadata_startup_script = <<SCRIPT
    yum -y install nfs-utils
    systemctl start nfs-server rpcbind
    systemctl enable nfs-server rpcbind
    mkdir -p "/home"
    mkdir -p "/tools"
    chmod 777 "/home" "/tools"
    echo '/home/ *(rw,sync,no_root_squash)' >> "/etc/exports"
    echo '/tools/ *(rw,sync,no_root_squash)' >> "/etc/exports"
    exportfs -r
  SCRIPT

  boot_disk {
    auto_delete = var.auto_delete_disk
    source      = google_compute_disk.default.name
  }

  network_interface {
    network = var.network_name
  }
  labels = var.labels
}
