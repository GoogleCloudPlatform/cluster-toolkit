output "server_ip" {
  description = "Webserver IP Address"
  value       = google_compute_instance.server_vm.network_interface[0].access_config[0].nat_ip
}

