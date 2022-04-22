variable "project" {
  description = "Project in which the VPC will be created."
  type        = string
}

variable "region" {
  description = "The region where Cloud NAT and Cloud Router will be configured."
  type        = string
}
