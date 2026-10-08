<!--
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Cloud Build Module

Executes Cloud Build jobs using custom `cloudbuild.yaml` template files (`.tfpl`), raw YAML strings, or repository build configs via `gcloud builds submit`. It supports both **Remote Git Repositories** (submitted via `--no-source` with Git cloning executed directly inside the Cloud Build runner, or combined with a local overlay via `target_dir` and `is_target_dir_local = true`) and **Local Filesystem Workspaces** (submitted with local directory packaging), dynamic Terraform apply-time template rendering (`template_vars`), Cloud Build runtime substitutions (`substitutions`), custom service accounts, and GCS staging buckets.

> **Note on Destroy Behavior:** Running `terraform destroy` removes `terraform_data.build` from the Terraform state without deleting the built container images in Artifact Registry or staged source archives in `gcs_staging_dir` (unless the underlying Artifact Registry repository or GCS bucket is also destroyed).

---

## Architecture & Workflow

```text
+-----------------------------------------------------------------------------------------+
| Blueprint Input:                                                                        |
|   cloud_build_dir:           "src" (workspace or repo subfolder)                        |
|   target_dir:                "overlays/custom-config" (repo or local path)              |
|   is_target_dir_local:       false (set true when target_dir is on the local machine)   |
|   cloud_build_template_path: $(ghpc_stage("./cloudbuild.yaml.tfpl"))                    |
|   repo_url:                  "https://github.com/my-org/my-repo.git" (optional)         |
|   repo_ref:                  "main" (branch, release tag, or commit SHA)               |
|   template_vars:             { _PROJECT_ID: "my-proj", _REGION: "us-central1" }         |
|   substitutions:             { _COMMIT_SHA: "a1b2c3d4" }                                |
+-----------------------------------------------------------------------------------------+
                                             |
                                             v
+-----------------------------------------------------------------------------------------+
| 1. Dynamic Template Rendering                                                           |
|    - Evaluates templatefile(cloud_build_template_path, template_vars)                   |
|    - Automatically injects _REPO_URL, _REPO_REF, _CLOUD_BUILD_DIR, and _TARGET_DIR      |
+-----------------------------------------------------------------------------------------+
                                             |
                                             v
+-----------------------------------------------------------------------------------------+
| 2. Cloud Build Execution via gcloud builds submit                                       |
|    - Remote Git Mode (repo_url set):   gcloud builds submit --no-source --config=...    |
|      (Cloud Build runner executes Step 0 `git clone` remotely in the cloud)             |
|    - Local Source Mode (repo_url null): gcloud builds submit "$CLOUD_BUILD_DIR" ...     |
+-----------------------------------------------------------------------------------------+
                                             |
                                             v
+-----------------------------------------------------------------------------------------+
| 3. Automatic Cleanup                                                                    |
|    - Ephemeral config in /tmp deleted via exit trap. Zero disk residue.                 |
+-----------------------------------------------------------------------------------------+
```

---

## Prerequisites & Requirements

This module executes locally during `terraform apply` via `scripts/cloud_build.sh` and requires:

- **`terraform`**: `>= 1.12.2`
- **`gcloud`**: Google Cloud SDK with `gcloud builds` support (`>= 400.0.0`)
- **`bash`**: Bash shell with standard utilities (`printf`, `mktemp`, `trap`, `cp`)

---

## Features

- **Remote Git Mode (`--no-source`)**: When `repo_url` is provided, Cloud Build is submitted with `--no-source` (or overlaid with `target_dir` when `is_target_dir_local = true`). Git checkout happens remotely in the cloud runner.
- **Local Directory Mode**: When `repo_url` is omitted, the module uploads the local source directory to Cloud Build.
- **Artifact Registry Image Check (`skip_if_exists`)**: Accepts a list of full container image URLs to check in Artifact Registry before submitting the build. If all target container images already exist, the build submission is skipped entirely. This feature does not detect content changes, and should only be used with immutable or versioned/content-hashed tags (e.g., `:sha256-...` or `:v1.0.0`).
- **Dynamic Templating (`template_vars`)**: Pass arbitrary key-value pairs into `templatefile()` for plan/apply-time variable resolution across Terraform modules.
- **Runtime Substitutions (`substitutions`)**: Pass `--substitutions=_KEY=VALUE` to Cloud Build for dynamic build execution variables.
- **Zero Disk Clutter & Pure Idempotency**: Temporary files are created in `/tmp` and removed upon exit, ensuring 0 plan diffs when re-running `terraform plan`.

---

## Usage Examples

### Example 1: Remote Git Build (Recommended)

Build directly from a remote Git repository:

```yaml
- group: cloud-build
  modules:
  - id: cloud-build
    source: community/modules/container/cloud-build
    settings:
      project_id: $(vars.project_id)
      region: $(vars.region)
      repo_url: "https://github.com/my-org/my-repo.git"
      repo_ref: "main"
      cloud_build_dir: "src"
      target_dir: "overlays/custom-config"
      is_target_dir_local: false
      cloud_build_template_path: $(ghpc_stage("./cloudbuild.yaml.tfpl"))
      service_account_email: $(vars.cloud_build_service_account_email)
      # Must use immutable or versioned tags (e.g. :v1.0), never mutable tags like :latest
      skip_if_exists:
        - "us-central1-docker.pkg.dev/my-proj/my-repo/my-image:v1.0"
      template_vars:
        _PROJECT_ID: $(vars.project_id)
        _REGION: $(vars.region)
        _REPO_NAME: $(artifact-repository.artifact_name)
        _IMAGE_NAME: "my-image"
```

### Example 2: Local Source Workspace Build

Build from local source code on disk:

```yaml
- group: cloud-build
  modules:
  - id: cloud-build
    source: community/modules/container/cloud-build
    settings:
      project_id: $(vars.project_id)
      region: $(vars.region)
      cloud_build_dir: $(ghpc_stage("./my-app"))
      cloud_build_template_path: $(ghpc_stage("./cloudbuild.yaml.tfpl"))
      service_account_email: $(vars.cloud_build_service_account_email)
      template_vars:
        _PROJECT_ID: $(vars.project_id)
        _REGION: $(vars.region)
        _REPO_NAME: $(artifact-repository.artifact_name)
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.10.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [terraform_data.build](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_cloud_build_content"></a> [cloud\_build\_content](#input\_cloud\_build\_content) | Raw YAML content for Cloud Build config (alternative to cloud\_build\_template\_path). | `string` | `null` | no |
| <a name="input_cloud_build_dir"></a> [cloud\_build\_dir](#input\_cloud\_build\_dir) | Directory containing source code for Cloud Build. When repo\_url is set, this is the relative subfolder in the remote repository (e.g. 'src'). When repo\_url is omitted, this is the local directory uploaded to Cloud Build. | `string` | `"."` | no |
| <a name="input_cloud_build_template_path"></a> [cloud\_build\_template\_path](#input\_cloud\_build\_template\_path) | Path to the cloudbuild.yaml template file (.tfpl or .yaml) to be rendered with template\_vars. | `string` | `null` | no |
| <a name="input_gcs_staging_dir"></a> [gcs\_staging\_dir](#input\_gcs\_staging\_dir) | Optional GCS bucket path for Cloud Build source staging (e.g. gs://bucket/staging). | `string` | `null` | no |
| <a name="input_is_target_dir_local"></a> [is\_target\_dir\_local](#input\_is\_target\_dir\_local) | Whether target\_dir resides on the local machine. When true, target\_dir is staged from the local filesystem and uploaded to Cloud Build. When false, target\_dir is assumed to reside inside the remote Git repository. | `bool` | `false` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID where Cloud Build executes. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | GCP region where Artifact Registry and Cloud Build execute. | `string` | `"us-central1"` | no |
| <a name="input_repo_ref"></a> [repo\_ref](#input\_repo\_ref) | Git branch name, release tag, or commit SHA to check out when repo\_url is specified. | `string` | `"main"` | no |
| <a name="input_repo_url"></a> [repo\_url](#input\_repo\_url) | Optional Git repository URL (e.g. https://github.com/my-org/my-repo.git). When specified, Cloud Build runs with --no-source (unless target\_dir is provided with is\_target\_dir\_local = true) and the repository is cloned directly within the Cloud Build pipeline. | `string` | `null` | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | Optional service account email to execute Cloud Build. | `string` | `null` | no |
| <a name="input_skip_if_exists"></a> [skip\_if\_exists](#input\_skip\_if\_exists) | List of container image URLs to check in Artifact Registry before building. If all images exist, the build is skipped. Note that this feature only checks if the image/tag exists in Artifact Registry, it does not detect content changes. Only use with immutable or versioned/content-hashed tags (e.g., :v1.0.0 or :sha256-...), and never with mutable tags like :latest. | `list(string)` | `[]` | no |
| <a name="input_substitutions"></a> [substitutions](#input\_substitutions) | Map of Cloud Build substitution variables (\_KEY=VALUE). | `map(string)` | `{}` | no |
| <a name="input_target_dir"></a> [target\_dir](#input\_target\_dir) | Optional path to a target or overlay directory (either inside the remote Git repository or on the local machine, controlled by is\_target\_dir\_local). Automatically exposed as \_TARGET\_DIR in template\_vars. | `string` | `null` | no |
| <a name="input_template_vars"></a> [template\_vars](#input\_template\_vars) | Map of variables to pass into templatefile when rendering cloud\_build\_template\_path. | `map(any)` | `{}` | no |
| <a name="input_triggers"></a> [triggers](#input\_triggers) | A map of arbitrary strings or outputs that, when changed, force this module to re-run. | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_id"></a> [id](#output\_id) | The unique execution ID of the container build. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
