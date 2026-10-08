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

output "bucket_name" {
  description = "Target Google Cloud Storage bucket name."
  value       = var.bucket_name
}

output "gcs_uris" {
  description = "Map of destination object paths to their full GCS URIs (gs://bucket/path)."
  value = {
    for k, obj in google_storage_bucket_object.objects :
    k => "gs://${obj.bucket}/${obj.name}"
  }
}

output "all_staged_uris" {
  description = "List of all GCS URIs for objects managed by this module."
  value = [
    for obj in google_storage_bucket_object.objects :
    "gs://${obj.bucket}/${obj.name}"
  ]
}

output "objects" {
  description = "Map of created storage bucket object resources with metadata."
  value = {
    for k, obj in google_storage_bucket_object.objects :
    k => {
      name    = obj.name
      bucket  = obj.bucket
      crc32c  = obj.crc32c
      md5hash = obj.md5hash
      gcs_uri = "gs://${obj.bucket}/${obj.name}"
    }
  }
}
