output "local_peering_name" {
  description = "The name of the local-to-remote peering."
  value       = google_compute_network_peering.local_to_remote.name
}

output "remote_peering_name" {
  description = "The name of the remote-to-local peering (if created)."
  value       = try(google_compute_network_peering.remote_to_local[0].name, null)
}
