variable "current_version" {
  type        = string
  description = "The version string to evaluate (e.g. 1.35.2-gke, v0.15.2, sha256-123)."
}

variable "minimum_version" {
  type        = string
  description = "The minimum required version (e.g. 1.35.0)."

  validation {
    condition     = can(regex("^[vV]?([0-9]+)(?:\\.([0-9]+))?(?:\\.([0-9]+))?(?:-gke\\.([0-9]+))?(?:[-+].*)?$", var.minimum_version))
    error_message = "The minimum_version must be a valid major.minor.patch[-gke.X] string."
  }
}