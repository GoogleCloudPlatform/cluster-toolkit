/**
 * Copyright 2023 Google LLC
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

variable "install_nvidia_driver" {
  description = "Install NVIDIA GPU drivers and the CUDA Toolkit using script specified by var.install_nvidia_driver_script"
  type        = bool
  default     = false
}

variable "install_nvidia_driver_script" {
  description = "Install script for NVIDIA drivers specified by http/https URL"
  type        = string
  default     = "https://developer.download.nvidia.com/compute/cuda/12.1.1/local_installers/cuda_12.1.1_531.14_windows.exe"
}

variable "install_nvidia_driver_args" {
  description = "Arguments to supply to NVIDIA driver install script"
  type        = string
  default     = "/s /n"
}

variable "http_proxy" {
  description = "Set system default web (http and https) proxy for use by Invoke-WebRequest"
  type        = string
  default     = ""
  nullable    = false
}
