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

variable "skills_eval_service_account" {
  description = "Optional least-privilege service account resource name (projects/<project>/serviceAccounts/<email>) for live agent skill evaluation triggers. If null, the default Cloud Build service account is used."
  type        = string
  default     = null
}

resource "google_cloudbuild_trigger" "pr_skills_smoke_eval" {
  name            = "PR-skills-smoke-eval"
  description     = "Run offline and 5-case live smoke evaluation on skill and evaluation runner changes"
  service_account = var.skills_eval_service_account

  filename = "tools/cloud-build/pr-skills-live-eval.yaml"

  github {
    owner = "GoogleCloudPlatform"
    name  = "cluster-toolkit"
    pull_request {
      branch          = ".*"
      comment_control = "COMMENTS_ENABLED_FOR_EXTERNAL_CONTRIBUTORS_ONLY"
    }
  }

  included_files = [
    "AGENTS.md",
    "skills/**",
    "tools/run_eval.py",
    "tools/tests/test_run_eval.py",
    "tools/cloud-build/pr-skills-live-eval.yaml",
  ]

  ignored_files = [
    "skills/**/references/catalog.yaml",
  ]

  substitutions = {
    _EVAL_CASES       = "case_1,case_4,case_6,case_15,case_16"
    _EVAL_CONCURRENCY = "4"
  }

  include_build_logs = "INCLUDE_BUILD_LOGS_WITH_STATUS"
}

resource "google_cloudbuild_trigger" "pr_test_skills_full_eval" {
  name            = "PR-test-skills-full-eval"
  description     = "Run the full live agent skills evaluation suite on demand against a PR"
  service_account = var.skills_eval_service_account

  filename = "tools/cloud-build/pr-skills-live-eval.yaml"
  approval_config {
    approval_required = true
  }

  github {
    owner = "GoogleCloudPlatform"
    name  = "cluster-toolkit"
    pull_request {
      branch          = ".*"
      comment_control = "COMMENTS_ENABLED_FOR_EXTERNAL_CONTRIBUTORS_ONLY"
    }
  }

  substitutions = {
    _EVAL_CASES       = "all"
    _EVAL_CONCURRENCY = "4"
  }

  include_build_logs = "INCLUDE_BUILD_LOGS_WITH_STATUS"
}
