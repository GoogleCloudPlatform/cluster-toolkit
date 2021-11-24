variable "project_id" {

}

variable "zone" {
  default = "us-central1-f"
}

variable "disk_size" {
  default = "100"
}

variable "type" {
  default = "pd-ssd"
}
variable "image_family" {
  type    = string
  default = "centos-7"
}

variable "source_image_project" {
  type    = string
  default = "centos-cloud"
}

variable "auto_delete_disk" {
  description = "Whether or not the boot disk should be auto-deleted"
  default     = false
}

variable "region" {
  description = "Region where the instances should be created."
  default     = "us-central1"
}

variable "network_name" {
  description = "Network to deploy to. Only one of network or subnetwork should be specified."
  default     = "default"
}

variable "name_prefix" {
  description = "The name prefix for the resources."
  default     = "hpc-nfs"
}

variable "machine_type" {
  description = "Type of the VM instance to use"
  default     = "n2d-standard-2"
}

variable "network_tier" {
  description = "IP Address Network Tier"
  default     = "PREMIUM"
}

variable "export_paths" {
  description = "Paths to exports"
  default     = ["/home/", "/tools"]
}

output "tools-volume-ip-addresses" {
  value = google_compute_instance.compute_instance.network_interface.0.network_ip
}

output "home-volume-ip-addresses" {
  value = google_compute_instance.compute_instance.network_interface.0.network_ip
}

variable "labels" {
  description = "Labels to add to the filestore instance. List key, value pairs."
  type        = any
}