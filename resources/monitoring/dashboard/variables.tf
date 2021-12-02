variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "deployment_name" {
  description = "The name of the current deployment"
  type        = string
}

variable "base_dashboard" {
  description = "Baseline dashboard template, either custom or from ./dashboards"
  type        = string
  default     = "HPC"
}

variable "widgets" {
  description = "List of additional widgets to add to the base dashboard."
  type        = list(string)
  default     = []
}
