// Use an external disk so that it can be remounted on another instance.
resource "google_compute_disk" "default" {
  name  = "${var.name_prefix}-disk"
  image = var.image_family
  size  = var.disk_size
  type  = var.type
  zone  = var.zone
}

resource "google_compute_instance" "compute_instance" {
  project                 = var.project_id
  name                    = "${var.name_prefix}-instance"
  zone                    = var.zone
  machine_type            = var.machine_type
  metadata_startup_script =  <<SCRIPT
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