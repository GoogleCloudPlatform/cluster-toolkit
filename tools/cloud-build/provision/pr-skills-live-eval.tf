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

resource "google_cloudbuild_trigger" "pr_skills_smoke_eval" {
  name        = "PR-skills-smoke-eval"
  description = "Run offline and 5-case live smoke evaluation on skill and validator changes"

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
    "cmd/explain.go",
    "pkg/config/**",
    "pkg/validators/**",
    "pkg/modulereader/**",
    "modules/**/metadata.yaml",
    "community/modules/**/metadata.yaml",
  ]

  substitutions = {
    _EVAL_CASES       = "case_1,case_4,case_6,case_10,case_11"
    _EVAL_CONCURRENCY = "3"
  }

  include_build_logs = "INCLUDE_BUILD_LOGS_WITH_STATUS"
}

resource "google_cloudbuild_trigger" "pr_test_skills_full_eval" {
  name        = "PR-test-skills-full-eval"
  description = "Run the full 17-case live agent skills evaluation suite on demand against a PR"

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
    _EVAL_CONCURRENCY = "3"
  }

  include_build_logs = "INCLUDE_BUILD_LOGS_WITH_STATUS"
}
