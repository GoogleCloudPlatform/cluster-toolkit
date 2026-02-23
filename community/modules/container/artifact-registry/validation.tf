# Copyright 2026 Google LLC
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

resource "terraform_data" "input_validation" {
  lifecycle {
    precondition {
      condition = (
        var.repo_password == null ||
        (var.use_upstream_credentials && var.repo_mode == "REMOTE_REPOSITORY")
      )
      error_message = "repo_password may be set only when repo_mode=REMOTE_REPOSITORY and use_upstream_credentials=true."
    }

    precondition {
      condition = (
        !var.use_upstream_credentials ||
        var.repo_mode == "REMOTE_REPOSITORY"
      )
      error_message = "use_upstream_credentials is allowed only when repo_mode is REMOTE_REPOSITORY."
    }

    precondition {
      condition = (
        var.repo_mode != "REMOTE_REPOSITORY" ||
        (var.repo_public_repository != null || var.repo_mirror_url != null)
      )
      error_message = "For a REMOTE_REPOSITORY you must set repo_public_repository or repo_mirror_url."
    }

    precondition {
      condition = (
        !contains(["APT", "YUM"], var.format) ||
        (var.repository_base != null && var.repository_path != null)
      )
      error_message = "APT/YUM formats require repository_base and repository_path."
    }
  }
}
