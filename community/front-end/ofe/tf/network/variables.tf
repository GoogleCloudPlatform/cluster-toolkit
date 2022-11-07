variable "project_id" {
  type        = string
  description = "Project in which to deploy"
}

variable "region" {
  type        = string
  description = "Region in which to deploy"
}


variable "deployment_name" {
  description = "Base \"name\" for the deployment."
  type        = string
}
