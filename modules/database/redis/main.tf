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

resource "google_project_service" "redis_api" {
  count              = var.deploy_redis ? 1 : 0
  project            = var.project_id
  service            = "redis.googleapis.com"
  disable_on_destroy = false
}
resource "google_redis_instance" "default" {
  count              = var.deploy_redis ? 1 : 0
  project            = var.project_id
  name               = "${var.deployment_name}-redis-${var.environment}"
  tier               = var.tier
  memory_size_gb     = var.memory_size_gb
  region             = var.region
  redis_version      = var.redis_version
  auth_enabled       = var.auth_enabled
  connect_mode       = var.connect_mode
  reserved_ip_range  = var.reserved_ip_range
  authorized_network = var.network_self_link
  depends_on         = [google_project_service.redis_api]
}
