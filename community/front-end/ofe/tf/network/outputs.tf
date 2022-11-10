output "subnet_name" {
  value       = google_compute_subnetwork.hosting_subnetwork.name
  description = "Name of the generated subnet"
}
