variable "project_id" {
  description = "ID of the Project"
  type        = string
}

variable "folder_id" {
  description = "ID of the Folder"
  type        = string
}

variable "billing_account" {
  description = "Account used to pay the bills"
  type        = string
}
variable "default_service_account" {
  description = "Project default service account setting: can be one of `delete`, `deprivilege`, `disable`, or `keep`."
  default     = "keep"
  type        = string
}

variable "org_id" {
  description = "ID of the organization"
  type        = string
}
