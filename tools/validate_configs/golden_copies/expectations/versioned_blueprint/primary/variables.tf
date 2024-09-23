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

variable "deployment_name" {
  description = "Toolkit deployment variable: deployment_name"
  type        = string
}

variable "gpu_zones" {
  description = "Toolkit deployment variable: gpu_zones"
  type        = list(any)
}

variable "instance_image_custom" {
  description = "Toolkit deployment variable: instance_image_custom"
  type        = bool
}

variable "labels" {
  description = "Toolkit deployment variable: labels"
  type        = any
}

variable "project_id" {
  description = "Toolkit deployment variable: project_id"
  type        = string
}

variable "region" {
  description = "Toolkit deployment variable: region"
  type        = string
}

variable "slurm_image" {
  description = "Toolkit deployment variable: slurm_image"
  type        = any
}

variable "zone" {
  description = "Toolkit deployment variable: zone"
  type        = string
}
