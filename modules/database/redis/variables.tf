/**
 * Copyright 2026 Google LLC
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

variable "project_id" {
  description = "The project ID to deploy to."
  type        = string
}
variable "environment" {
  description = "The environment name."
  type        = string
}
variable "redis_region" {
  description = "The region to deploy Redis to."
  type        = string
}
variable "deploy_redis" {
  description = "Whether to deploy Redis."
  type        = bool
  default     = true
}
variable "authorized_network" {
  description = "The VPC network to which the instance is connected."
  type        = string
}
variable "connect_mode" {
  description = "The connection mode of the Redis instance."
  type        = string
  default     = "DIRECT_PEERING"
}
variable "reserved_ip_range" {
  description = "The name of the allocated IP range for the Private Service Access."
  type        = string
  default     = null
}
