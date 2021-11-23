variable "project_id" {
  type    = string  
}

variable "folder_id" {
  type  = string
}

variable "billing_account" {
  type  = string
}
variable "default_service_account" {
  description = "Project default service account setting: can be one of `delete`, `deprivilege`, `disable`, or `keep`."
  default     = "keep"
  type        = string
}
