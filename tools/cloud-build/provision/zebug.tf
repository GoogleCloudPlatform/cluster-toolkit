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

resource "google_cloudbuild_trigger" "zebug_fast_build_failure" {
  name        = "ZEBUG-fast-build-failure"
  description = "A build that always fails fast"
  tags        = [local.notify_chat_tag]

  build {
    step {
      name = "busybox"
      args = ["false"]
    }
  }

  source_to_build {
    uri       = var.repo_uri
    ref       = local.ref_main
    repo_type = "GITHUB"
  }
}

resource "google_cloudbuild_trigger" "zebug_fast_build_success" {
  name        = "ZEBUG-fast-build-success"
  description = "A build that always succeeds fast"

  build {
    step {
      name = "busybox"
      args = ["true"]
    }
  }

  source_to_build {
    uri       = var.repo_uri
    ref       = local.ref_main
    repo_type = "GITHUB"
  }
}
