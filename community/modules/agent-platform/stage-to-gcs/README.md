# GCS Staging Module (`stage-to-gcs`)

A generic, reusable Cluster Toolkit / Terraform module for staging and synchronizing arbitrary directories and individual files into Google Cloud Storage (GCS) buckets.

This module provides a unified interface supporting **both local filesystem paths and remote Git repositories**, enabling complete decoupling between deployment blueprints and underlying framework or workload source code.

---

## Prerequisites & Requirements

This module executes locally during `terraform apply` via a pure POSIX shell provisioner and requires:

* **`terraform`**: `>= 1.4.0` (required for `terraform_data` resource and optional object attribute defaults)
* **`git`**: `>= 2.28` (required for cone-mode sparse-checkout and `--skip-checks` support)
* **`gcloud`**: Google Cloud SDK with `gcloud storage` support (`>= 400.0.0`)
* **POSIX Utilities**: `bash`, `base64`, `printf`, `mktemp`, `trap`, and standard hashing tools (`sha256sum`, `shasum`, or `md5sum`)

---

## How It Works

```text
+--------------------------------------------------------------------------------+
| Blueprint Input:                                                               |
|   directories: [source_path: "examples/my_benchmark", destination_path: ...]   |
|   files:       [source_path: "configs/requirements.txt", destination_path: ...] |
|   repo_url:    "https://github.com/my-org/my-repo.git" (optional)              |
|   repo_ref:    "main" (branch, tag, or full/short commit SHA)                  |
+--------------------------------------------------------------------------------+
                                      |
                                      v
+--------------------------------------------------------------------------------+
| 1. Ephemeral /tmp Workspace Allocation                                         |
|    - Allocates /tmp/tmp.XXXXXX and registers a POSIX cleanup trap:            |
|      trap "rm -rf '$TMP_GIT_DIR'" EXIT                                         |
+--------------------------------------------------------------------------------+
                                      |
                                      v
+--------------------------------------------------------------------------------+
| 2. Shallow, Blobless, Sparse Git Fetch (Branch, Tag, or Commit SHA)            |
|    - Initializes sparse cone mode repository                                  |
|    - Fetches exact ref snapshot: git fetch --depth 1 origin <repo_ref>         |
|    - Checks out only target paths: git sparse-checkout add <path>              |
+--------------------------------------------------------------------------------+
                                      |
                                      v
+--------------------------------------------------------------------------------+
| 3. GCS Staging & Exclusion Filtering                                           |
|    - Directories: gcloud storage rsync -r -x "<exclude_regex>" <src> <dest_uri>|
|    - Files:       gcloud storage cp <src> <dest_uri>                           |
+--------------------------------------------------------------------------------+
                                      |
                                      v
+--------------------------------------------------------------------------------+
| 4. Automatic Workspace Teardown                                                |
|    - POSIX EXIT trap immediately removes /tmp workspace. Zero disk residue.    |
+--------------------------------------------------------------------------------+
```

### Key Technical Mechanisms

1. **Deterministic Plan Idempotency**:
   * Staging triggers are calculated deterministically from inputs (`bucket_name`, `repo_url`, `repo_ref`, `directories`, and `files`).
   * `terraform plan` produces zero diff when blueprint configuration and file contents are unchanged.
   * To force a re-sync without changing inputs, use `terraform apply -replace='module.<id>.terraform_data.stage_gcs_file["<key>"]'`.

2. **Full Git Ref Support (Branches, Tags, and Commit SHAs)**:
   * Supports branch names, release tags, and full/short commit SHAs.
   * Attempts shallow ref fetch (`git fetch --depth 1`) first, and falls back to blobless graph fetch (`git fetch --filter=blob:none origin`) for Git remotes that restrict direct shallow SHA queries. File contents (blobs) are still only downloaded for sparse checkout paths.

3. **True Sparse & Blobless Ingestion**:
   * **Shallow (`--depth 1`)**: Discards all historical git commits and branches.
   * **Blobless (`--filter=blob:none`)**: Downloads repository tree metadata without downloading file bodies (blobs).
   * **Sparse Checkout (`git sparse-checkout add`)**: Only downloads the contents of the exact folders or files specified in the blueprint. Unrelated folders in large repositories are never downloaded.

4. **Multi-Item Caching & Cross-Platform Portability**:
   * Dynamically detects hashing tools (`sha256sum`, `shasum`, `md5sum`, `md5`, `cksum`) for cross-platform compatibility across Linux, macOS, and containerized CI/CD runners.
   * Reuses the initialized sparse repository across all folder and file entries in a single deployment.

5. **Dynamic Local vs. Remote Path Resolution**:
   * When `repo_url` is omitted or empty, the module resolves local filesystem paths.
   * Because Cluster Toolkit (`gcluster`) executes Terraform from within isolated deployment subdirectories (e.g. `-o <outdir>/<deployment_name>/<group>`), the module first checks if the path exists directly, and if not, performs an upward directory traversal from `pwd -P` to locate the workspace root. This ensures that workspace-relative paths (like `examples/...` or `configs/...`) work reliably regardless of the deployment output location.

6. **Regex Exclusion Filtering**:
   * Supports configurable regex exclusions (defaulting to `[".*Dockerfile$"]`), ensuring build artifacts and container recipes (such as `Dockerfile` or `*.Dockerfile`) are not uploaded to runtime storage mounts.

---

## Example Usage

### 1. Staging an Entire Directory

Syncs an entire directory (e.g. `examples/my_workload` or `./my_local_data`) to `gs://<bucket>/workload/source/` while excluding `.Dockerfile`:

```yaml
# Stage directory into GCS
- group: source-staging
  modules:
  - id: stage-source
    source: community/modules/agent-platform/stage-to-gcs
    settings:
      bucket_name: $(vars.bucket_name)
      repo_url: $(vars.repo_url)     # Leave empty for local code, or pass Git repository URL
      repo_ref: $(vars.repo_ref)     # Git branch, tag, or commit SHA
      directories:
        - source_path: $(vars.source_dir)
          destination_path: "workload/source"
          exclude: [".*Dockerfile$"]
```

---

### 2. Staging Configuration Files & Notebooks

Uploads specific configuration files, batch definitions, and Jupyter notebooks to dedicated GCS object locations:

```yaml
# Stage static configuration files and notebooks into GCS
- group: staging-files
  modules:
  - id: stage-files
    source: community/modules/agent-platform/stage-to-gcs
    settings:
      bucket_name: $(vars.bucket_name)
      repo_url: $(vars.repo_url)
      repo_ref: $(vars.repo_branch)
      files:
        - source_path: "configs/requirements.txt"
          destination_path: "config/requirements.txt"

        - source_path: "configs/batch-job.yaml"
          destination_path: "config/batch-job.yaml"

        - source_path: "notebooks/analysis.ipynb"
          destination_path: "notebooks/analysis.ipynb"
```

---

### 3. Mixed / Multi-Source Staging

Combines both directory and file uploads, overriding the Git repository per item:

```yaml
- group: staging
  modules:
  - id: stage-assets
    source: community/modules/agent-platform/stage-to-gcs
    settings:
      bucket_name: $(vars.bucket_name)
      directories:
        # Directory from Git repo locked to a commit SHA
        - source_path: "benchmarks/benchmark_suite"
          destination_path: "benchmarks/run1"
          repo_url: "https://github.com/my-org/my-benchmarks.git"
          repo_ref: "00bbd75fd83579c17d43469ab48f911078841013"
          exclude: [".*Dockerfile$", ".*\\.pyc$"]

        # Directory from local disk
        - source_path: "./local_dataset"
          destination_path: "datasets/v1"

      files:
        # File from local disk
        - source_path: "./configs/custom_settings.yaml"
          destination_path: "config/settings.yaml"
```

---

## Object Schemas

### Directory Object Schema

| Field | Description | Type | Default |
| :--- | :--- | :--- | :--- |
| `source_path` | Local directory path or directory path relative to the Git repository root. | `string` | **Required** |
| `destination_path` | Target directory prefix in the GCS bucket (e.g. `exp-1/source`). | `string` | **Required** |
| `repo_url` | Optional Git repository URL override for this directory. | `string` | `var.repo_url` |
| `repo_ref` | Optional Git branch, tag, or commit SHA override. | `string` | `var.repo_ref` |
| `exclude` | Regex patterns to exclude during `gcloud storage rsync`. | `list(string)` | `[".*Dockerfile$"]` |

### File Object Schema

| Field | Description | Type | Default |
| :--- | :--- | :--- | :--- |
| `source_path` | Local file path or file path relative to the Git repository root. | `string` | **Required** |
| `destination_path` | Target object path in the GCS bucket (e.g. `config/requirements.txt`). | `string` | **Required** |
| `repo_url` | Optional Git repository URL override for this file. | `string` | `var.repo_url` |
| `repo_ref` | Optional Git branch, tag, or commit SHA override. | `string` | `var.repo_ref` |

---

## License

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.4.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [terraform_data.stage_gcs_directory](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.stage_gcs_file](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | Name of the target Google Cloud Storage bucket. | `string` | n/a | yes |
| <a name="input_directories"></a> [directories](#input\_directories) | List of directories to stage/sync to the GCS bucket. Each entry defines a source directory (local or remote Git) and a destination path in the bucket. | <pre>list(object({<br/>    source_path      = string<br/>    destination_path = string<br/>    repo_url         = optional(string)<br/>    repo_ref         = optional(string)<br/>    exclude          = optional(list(string), [".*Dockerfile$"])<br/>  }))</pre> | `[]` | no |
| <a name="input_files"></a> [files](#input\_files) | List of individual files to stage/upload to the GCS bucket. Each entry defines a source file (local or remote Git) and a destination object path in the bucket. | <pre>list(object({<br/>    source_path      = string<br/>    destination_path = string<br/>    repo_url         = optional(string)<br/>    repo_ref         = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_repo_ref"></a> [repo\_ref](#input\_repo\_ref) | Default Git branch, commit, or tag to checkout when using Git repositories. | `string` | `"main"` | no |
| <a name="input_repo_url"></a> [repo\_url](#input\_repo\_url) | Default Git repository URL to clone files/directories from. If omitted or null, local filesystem paths are used by default. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_all_staged_uris"></a> [all\_staged\_uris](#output\_all\_staged\_uris) | Combined list of all destination GCS URIs managed by this module. |
| <a name="output_bucket_name"></a> [bucket\_name](#output\_bucket\_name) | Name of the target Google Cloud Storage bucket. |
| <a name="output_id"></a> [id](#output\_id) | A composite ID ensuring all staged files and directories have finished uploading. |
| <a name="output_staged_directory_uris"></a> [staged\_directory\_uris](#output\_staged\_directory\_uris) | List of destination GCS URIs (gs://...) for staged directories. |
| <a name="output_staged_file_uris"></a> [staged\_file\_uris](#output\_staged\_file\_uris) | List of destination GCS URIs (gs://...) for staged files. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
