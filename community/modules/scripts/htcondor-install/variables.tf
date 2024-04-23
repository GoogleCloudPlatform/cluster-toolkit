/**
 * Copyright 2022 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

variable "enable_docker" {
  description = "Install and enable docker daemon alongside HTCondor"
  type        = bool
  default     = true
}

variable "condor_version" {
  description = "Yum/DNF-compatible version string; leave unset to use latest 23.0 LTS release (examples: \"23.0.0\",\"23.*\"))"
  type        = string
  default     = "23.*"

  validation {
    error_message = "var.condor_version must be set to \"23.*\" for latest 23.0 release or to a specific \"23.0.y\" release."
    condition = var.condor_version == "23.*" || (
      length(split(".", var.condor_version)) == 3 && alltrue([
        for v in split(".", var.condor_version) : can(tonumber(v))
      ]) && split(".", var.condor_version)[0] == "23"
      && split(".", var.condor_version)[1] == "0"
    )
  }
}

variable "http_proxy" {
  description = "Set system default web (http and https) proxy for Windows HTCondor installation"
  type        = string
  default     = ""
  nullable    = false
}

variable "python_windows_installer_url" {
  description = "URL of Python installer for Windows"
  type        = string
  default     = "https://www.python.org/ftp/python/3.11.9/python-3.11.9-amd64.exe"
  nullable    = false
}
