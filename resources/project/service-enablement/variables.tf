variable "project_id" {
  description = "ID of the project"
  type        = string
}

variable "gcp_service_list" {
  description = "list of APIs to be enabled for the project"
  type        = list(string)
}
