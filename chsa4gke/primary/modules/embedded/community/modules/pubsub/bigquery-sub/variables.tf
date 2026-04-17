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
  description = "The name of the current deployment"
  type        = string
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "topic_id" {
  description = "The name of the pubsub topic to subscribe to. Can be provided by the pubsub/topic module"
  type        = string
}

variable "subscription_id" {
  description = "The name of the pubsub subscription to be created"
  type        = string
  default     = null
}

variable "dataset_id" {
  description = "Name of the dataset that was created. Can be provided by the bigquery-table module"
  type        = string
}

variable "table_id" {
  description = "ID of created BQ table. Can be provided by the bigquery-table module"
  type        = string
}

variable "labels" {
  description = "Labels to add to the instances. Key-value pairs."
  type        = map(string)
}
