/**
 * Copyright 2022 Google LLC
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
  description = "Bucket for PBS RPM packages"
  value       = module.pbspro_bucket.bucket.name
}

output "pbs_client_rpm_url" {
  description = "gsutil URL of PBS client RPM package"
  value       = "gs://${module.pbspro_bucket.bucket.name}/${google_storage_bucket_object.client_rpm.name}"
  depends_on = [
    google_storage_bucket_object.client_rpm,
  ]
}

output "pbs_devel_rpm_url" {
  description = "gsutil URL of PBS development RPM package"
  value       = "gs://${module.pbspro_bucket.bucket.name}/${google_storage_bucket_object.devel_rpm.name}"
  depends_on = [
    google_storage_bucket_object.devel_rpm,
  ]
}

output "pbs_execution_rpm_url" {
  description = "gsutil URL of PBS execution host RPM package"
  value       = "gs://${module.pbspro_bucket.bucket.name}/${google_storage_bucket_object.execution_rpm.name}"
  depends_on = [
    google_storage_bucket_object.execution_rpm,
  ]
}

output "pbs_server_rpm_url" {
  description = "gsutil URL of PBS server host RPM package"
  value       = "gs://${module.pbspro_bucket.bucket.name}/${google_storage_bucket_object.server_rpm.name}"
  depends_on = [
    google_storage_bucket_object.server_rpm,
  ]
}

output "pbs_license_file_url" {
  description = "gsutil URL of PBS license file"
  value       = try("gs://${module.pbspro_bucket.bucket.name}/${google_storage_bucket_object.license_file[0].name}", null)
}
