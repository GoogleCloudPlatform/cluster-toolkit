variable "deployment_name" {
  description = "The name of the current deployment"
  type        = string
}

variable "local_network_self_link" {
  description = "Self-link of the local network to be peered."
  type        = string
}

variable "remote_network_self_link" {
  description = "Self-link of the remote network to be peered."
  type        = string
}

variable "peering_name" {
  description = "Name of the local-to-remote peering. If null, a name will be generated."
  type        = string
  default     = null
}

variable "create_remote_peering" {
  description = "Whether to create the remote-to-local peering resource. Set to true if peering two networks in the same blueprint."
  type        = bool
  default     = false
}

variable "remote_peering_name" {
  description = "Name of the remote-to-local peering. If null, a name will be generated."
  type        = string
  default     = null
}

variable "export_custom_routes" {
  description = "Whether to export custom routes to the peer network."
  type        = bool
  default     = false
}

variable "import_custom_routes" {
  description = "Whether to import custom routes from the peer network."
  type        = bool
  default     = false
}

variable "import_subnet_routes_with_public_ip" {
  description = "Whether subnet routes with public IP range are imported."
  type        = bool
  default     = false
}

variable "stack_type" {
  description = "Which IP version(s) of traffic and routes are allowed to be imported or exported (e.g., IPV4_ONLY)."
  type        = string
  default     = "IPV4_ONLY"
}
