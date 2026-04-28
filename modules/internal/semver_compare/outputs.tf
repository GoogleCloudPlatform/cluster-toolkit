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

output "is_valid_semver" {
  value       = local.is_valid_semver
  description = "True if both versions could be parsed into major.minor semantic logic."
}

output "is_greater_than_or_equal" {
  value       = local.is_greater_than_or_equal
  description = "True if the version meets the minimum requirement, or if the version is a non-standard custom string (fail-open)."
}
