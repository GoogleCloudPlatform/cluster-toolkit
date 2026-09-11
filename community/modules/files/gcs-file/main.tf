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

resource "google_storage_bucket_object" "file" {
  name           = var.object_path
  bucket         = var.bucket_name
  content        = var.content
  source         = var.source_path
  content_type   = var.content_type
  detect_md5hash = var.source_path != null ? filemd5(substr(var.source_path, 0, 1) == "/" ? var.source_path : "${path.root}/${var.source_path}") : null
}
