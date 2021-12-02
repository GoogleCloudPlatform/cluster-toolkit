output "network_storage" {
  description = "Describes a nfs instance."
  value = {
    server_ip     = google_compute_instance.compute_instance.network_interface[0].network_ip
    remote_mount  = "/tools"
    local_mount   = "/tools"
    fs_type       = "nfs"
    mount_options = "defaults,hard,intr"
  }
}
