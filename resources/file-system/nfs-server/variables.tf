variable "deployment_name" {
  description = "Name of the HPC deployment, used as name of the filestore instace if no name is specified."
  type        = string
}

variable "zone" {
  description = "The name of the Filestore zone of the instance."
  type        = string
}

variable "disk_size" {
  description = "Storage size gb"
  type        = number
  default     = "100"
}

variable "type" {
  description = "The service tier of the instance."
  type        = string
  default     = "pd-ssd"
}

variable "image_family" {
  description = "the VM image used by the nfs server"
  type        = string
  default     = "centos-7"
}

variable "auto_delete_disk" {
  description = "Whether or not the boot disk should be auto-deleted"
  type        = bool
  default     = false
}

variable "network_name" {
  description = "Network to deploy to. Only one of network or subnetwork should be specified."
  type        = string
  default     = "default"
}

variable "machine_type" {
  description = "Type of the VM instance to use"
  type        = string
  default     = "n2d-standard-2"
}

variable "labels" {
  description = "Labels to add to the filestore instance. List key, value pairs."
  type        = any
}
