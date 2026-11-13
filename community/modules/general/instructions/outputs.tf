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

output "instructions" {
  description = "Formatted user instructions."
  value       = local.formatted_instructions
}

output "file_path" {
  description = "Path of the local instructions file, if write_to_file was configured."
  value       = local.file_destination != null ? local_file.instructions_file[0].filename : null
}
