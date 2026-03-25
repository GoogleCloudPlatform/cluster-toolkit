variable "current_version" {
  type        = string
  description = "The version string to evaluate (e.g. 1.35.2-gke, v0.15.2, sha256-123)."
}

variable "minimum_version" {
  type        = string
  description = "The minimum required version (e.g. 1.35.0)."
}