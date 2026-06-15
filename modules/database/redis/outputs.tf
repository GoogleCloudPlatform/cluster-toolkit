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

output "redis_host" {
  description = "The host of the Redis instance."
  value       = var.deploy_redis ? google_redis_instance.default[0].host : null
}
output "redis_port" {
  description = "The port of the Redis instance."
  value       = var.deploy_redis ? google_redis_instance.default[0].port : null
}
output "auth_string" {
  description = "The auth string (password) of the Redis instance."
  value       = var.deploy_redis ? google_redis_instance.default[0].auth_string : null
  sensitive   = true
}
