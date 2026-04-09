# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

locals {
  # Strip leading 'v' if present
  clean_version     = var.current_version != null ? trimprefix(var.current_version, "v") : ""
  clean_min_version = var.minimum_version != null ? trimprefix(var.minimum_version, "v") : ""

  # Regex to capture strictly major.minor.patch integers, and optionally a -gke.123 build suffix
  version_regex = "^([0-9]+)(?:\\.([0-9]+))?(?:\\.([0-9]+))?(?:-gke\\.([0-9]+))?(?:[-+].*)?$"

  # Try to parse. If it fails (e.g., 'sha256-12345'), it returns null.
  parsed_ver = try(regex(local.version_regex, local.clean_version), null)
  parsed_min = try(regex(local.version_regex, local.clean_min_version), null)

  is_valid_current = local.parsed_ver != null
  is_valid_min     = local.parsed_min != null

  is_valid_semver = local.is_valid_current && local.is_valid_min

  # Map to integers, defaulting to 0 for missing patch versions or gke build numbers
  ver_major = local.is_valid_current ? tonumber(local.parsed_ver[0]) : 0
  ver_minor = local.is_valid_current ? tonumber(coalesce(local.parsed_ver[1], "0")) : 0
  ver_patch = local.is_valid_current ? tonumber(coalesce(local.parsed_ver[2], "0")) : 0
  ver_gke   = local.is_valid_current ? tonumber(coalesce(local.parsed_ver[3], "0")) : 0

  min_major = local.is_valid_min ? tonumber(local.parsed_min[0]) : 0
  min_minor = local.is_valid_min ? tonumber(coalesce(local.parsed_min[1], "0")) : 0
  min_patch = local.is_valid_min ? tonumber(coalesce(local.parsed_min[2], "0")) : 0
  min_gke   = local.is_valid_min ? tonumber(coalesce(local.parsed_min[3], "0")) : 0

  # Fail-open logic for custom tags
  is_greater_than_or_equal = local.is_valid_min ? (
    !local.is_valid_current ||
    local.ver_major > local.min_major ||
    (local.ver_major == local.min_major && local.ver_minor > local.min_minor) ||
    (local.ver_major == local.min_major && local.ver_minor == local.min_minor && local.ver_patch > local.min_patch) ||
    (local.ver_major == local.min_major && local.ver_minor == local.min_minor && local.ver_patch == local.min_patch && local.ver_gke >= local.min_gke)
  ) : false
}
