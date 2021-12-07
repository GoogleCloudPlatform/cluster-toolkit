/**
 * Copyright 2021 Google LLC
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

variable "deployment_name" {
  description = "Name of the HPC deployment, used as name of the NFS instace if no name is specified."
  type        = string
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "name" {
  description = "The resource name of the instance."
  type        = string
  default     = null
}

variable "network_project" {
  description = "the project where the shared network located in"
  type        = string
  default     = "default"
}

variable "zone" {
  description = "The zone name where the nfs instance located in."
  type        = string
}

variable "disk_size" {
  description = "Storage size gb"
  type        = number
  default     = "100"
}

variable "type" {
  description = "The service tier of the instance."
  type        = string
  default     = "pd-ssd"
}

variable "image" {
  description = "the VM image used by the nfs server"
  type        = string
  default     = "cloud-hpc-image-public/hpc-centos-7"
}

variable "auto_delete_disk" {
  description = "Whether or not the nfs disk should be auto-deleted"
  type        = bool
  default     = false
}

variable "network_name" {
  description = "Network to deploy to. Only one of network or subnetwork should be specified."
  type        = string
  default     = "default"
}

variable "machine_type" {
  description = "Type of the VM instance to use"
  type        = string
  default     = "n2d-standard-2"
}

variable "labels" {
  description = "Labels to add to the NFS instance. List key, value pairs."
  type        = any
}

variable "metadata" {
  description = "Metadata, provided as a map"
  type        = map(string)
  default     = {}
}

variable "service_account" {
  description = "Service Account for the NFS Server"
  type        = string
  default     = null
}

variable "scopes" {
  description = "Scopes to apply to the controller"
  type        = list(string)
  default     = ["https://www.googleapis.com/auth/cloud-platform"]
}

variable "local_mounts" {
  description = "Mountpoint for this NFS compute instance"
  type        = list(string)
  default     = ["/tools", "/data"]
}
