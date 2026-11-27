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
  description = "Dedicated least-privilege service account resource name (projects/<project>/serviceAccounts/<email>) for live agent skill evaluation triggers (defaults to projects/<project_id>/serviceAccounts/skills-eval-runner@<project_id>.iam.gserviceaccount.com when null; requires project-level roles/aiplatform.user, roles/compute.viewer, roles/serviceusage.serviceUsageViewer, roles/serviceusage.serviceUsageConsumer, roles/logging.logWriter, repo-scoped roles/artifactregistry.reader on us-central1/hpc-toolkit-repo, and bucket-scoped roles/storage.objectUser plus roles/storage.legacyBucketReader on the Cloud Build logs and eval artifacts buckets; see provision/README.md)."
  type        = string
  default     = null
  validation {
    condition     = var.skills_eval_service_account == null || can(regex("^projects/[^/]+/serviceAccounts/[a-z][a-z0-9-]{4,28}[a-z0-9]@[a-z0-9-]+\\.iam\\.gserviceaccount\\.com$", var.skills_eval_service_account))
    error_message = "skills_eval_service_account must be null or a dedicated custom IAM service account resource name (projects/<project>/serviceAccounts/<name>@<project>.iam.gserviceaccount.com); default Compute Engine and Cloud Build service accounts are rejected."
  }
}

locals {
  skills_eval_service_account = coalesce(
    var.skills_eval_service_account,
    "projects/${var.project_id}/serviceAccounts/skills-eval-runner@${var.project_id}.iam.gserviceaccount.com",
  )
  skills_eval_included_files = [
    "AGENTS.md",
    "CLAUDE.md",
    "GEMINI.md",
    "docs/migration/xpk_to_clustertoolkit.md",
    "pkg/catalog/**",
    "skills/**",
    "tools/catalog-gen/**",
    "tools/run_eval.py",
    "tools/skills_eval/**",
    "tools/tests/test_run_eval.py",
    "tools/validate_readme_snippets.py",
    "tools/cloud-build/pr-skills-live-eval.yaml",
    "tools/cloud-build/images/test-runner/**",
    "tools/cloud-build/provision/pr-skills-live-eval.tf",
  ]
  skills_eval_ignored_files = [
    "skills/README.md",
    "skills/**/references/catalog.yaml",
    "tools/skills_eval/README.md",
    "tools/cloud-build/provision/README.md",
  ]
}

resource "google_cloudbuild_trigger" "pr_skills_smoke_eval" {
  name            = "PR-skills-smoke-eval"
  description     = "Run offline and live smoke evaluation on modified skills and evaluation runner changes"
  service_account = local.skills_eval_service_account

  filename = "tools/cloud-build/pr-skills-live-eval.yaml"

  github {
    owner = "GoogleCloudPlatform"
    name  = "cluster-toolkit"
    pull_request {
      branch          = ".*"
      comment_control = "COMMENTS_ENABLED_FOR_EXTERNAL_CONTRIBUTORS_ONLY"
    }
  }

  included_files = local.skills_eval_included_files
  ignored_files  = local.skills_eval_ignored_files

  substitutions = {
    _EVAL_CASES       = "smoke"
    _EVAL_CONCURRENCY = "4"
  }

  include_build_logs = "INCLUDE_BUILD_LOGS_WITH_STATUS"
}

resource "google_cloudbuild_trigger" "pr_test_skills_full_eval" {
  name            = "PR-test-skills-full-eval"
  description     = "Run the full live agent skills evaluation suite on demand against a PR"
  service_account = local.skills_eval_service_account

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

  included_files = concat(local.skills_eval_included_files, ["cmd/**", "pkg/**"])
  ignored_files  = local.skills_eval_ignored_files

  substitutions = {
    _EVAL_CASES       = "all"
    _EVAL_CONCURRENCY = "4"
  }

  include_build_logs = "INCLUDE_BUILD_LOGS_WITH_STATUS"
}
