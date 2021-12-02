variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "region" {
  description = "The region where SQL instance will be configured"
  type        = string
}

variable "tier" {
  description = "The machine type to use for the SQL instance"
  type        = string
}

variable "sql_instance_name" {
  description = "name given to the sql instance for ease of identificaion"
  type        = string
}

variable "nat_ips" {
  description = "a list of NAT ips to be allow listed for the slurm cluster communication"
  type        = list(any)
}

variable "deletion_protection" {
  description = "Whether or not to allow Terraform to destroy the instance."
  type        = string
  default     = false
}
