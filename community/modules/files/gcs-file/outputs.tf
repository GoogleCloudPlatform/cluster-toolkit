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

output "gcs_uri" {
  description = "The full GCS URI of the uploaded file."
  value       = "gs://${google_storage_bucket_object.file.bucket}/${google_storage_bucket_object.file.name}"
}

output "object_name" {
  description = "The GCS object name of the uploaded file."
  value       = google_storage_bucket_object.file.name
}

output "bucket" {
  description = "The target GCS bucket where the object was uploaded."
  value       = google_storage_bucket_object.file.bucket
}

output "crc32c" {
  description = "The CRC32C hash of the uploaded object content."
  value       = google_storage_bucket_object.file.crc32c
}

output "md5hash" {
  description = "The MD5 hash of the uploaded object content."
  value       = google_storage_bucket_object.file.md5hash
}
