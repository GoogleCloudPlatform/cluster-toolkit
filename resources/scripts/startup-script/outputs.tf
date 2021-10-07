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

output "startup_script_content" {
  description = "startup-script-stdlib.sh content as a string value."
  value       = module.startup_scripts.content
}

output "startup_script_custom_content" {
  description = "Custom startup script to load and run all runners."
  value = templatefile(
    "${path.module}/templates/startup-script-custom.tpl",
    {
      bucket = google_storage_bucket.configs_bucket.name,
      runners = [
        for p in var.runners : {
          object = basename(p.file), type = p.type
        }
      ]
    }
  )
}
