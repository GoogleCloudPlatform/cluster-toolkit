# GCS Sync Module

This module synchronizes files and directories from remote Git repositories into Google Cloud Storage (GCS) buckets at apply time using `local-exec` and `gcloud storage rsync` / `cp`.

## Why use this module rather than a CI step?

The primary justification for staging assets via this Terraform module is **DAG ordering**:

* Downstream infrastructure components (such as Cloud Batch jobs, Compute Engine VMs, Vertex AI Colab runtimes, and Slurm clusters) frequently depend on code, dependencies, or configuration staged in Cloud Storage.
* Wiring outputs from this module (such as `all_staged_uris` or `staged_directory_uris`) into downstream resources establishes strict dependency edges within Terraform's Directed Acyclic Graph (DAG).
* This guarantees that remote repositories are cloned, filtered, and synchronized to GCS *before* compute jobs or runtimes start executing, eliminating race conditions without requiring custom CI/CD pipeline orchestration.

> [!NOTE]
> For **local files, local directories, or inline content**, use the declarative companion module [gcs-objects](../gcs-objects/README.md). That module manages objects directly within Terraform state (`google_storage_bucket_object`), providing drift detection and automatic removal upon `terraform destroy`.

## Important Considerations

### 1. Cleanup Limitation (Unmanaged Objects)
Objects synchronized by this module are created out-of-band via `gcloud storage` and are **not** tracked in Terraform state. Therefore, running `terraform destroy` will **not** delete staged files or directories from the GCS bucket.

* **Recommended Mitigation**: Apply a Google Cloud Storage Bucket Lifecycle Rule (e.g., auto-delete objects under the staging prefix after $N$ days) or manage bucket lifecycle policies declaratively.
* **Why not a destroy provisioner?**: Terraform `when = destroy` provisioners are intentionally avoided here: they do not execute if a resource is tainted from an earlier failure, do not run when a module block is removed from configuration, and are disabled when `create_before_destroy` is active.

### 2. Pin `repo_ref` to a Commit SHA or Tag
Terraform detects changes via `triggers_replace`, which hashes the `repo_ref` string. If `repo_ref` is set to a mutable branch name such as `"main"`, subsequent runs will see no configuration change and will **not** re-fetch upstream commits. Always pin `repo_ref` to an immutable Git commit SHA or release tag for predictable updates.

### 3. Destination Path Scoping
Directory synchronization uses `gcloud storage rsync --delete-unmatched-destination-objects` to prevent stale upstream files from accumulating. To protect other bucket contents, `destination_path` is strictly validated and gated: it **cannot** target the root of the bucket (`/`), and must always specify a dedicated subpath prefix.

---

## Usage Examples

### Synchronizing a Directory from Git

```yaml
- group: staging
  modules:
  - id: stage-code
    source: community/modules/file-system/gcs-sync
    settings:
      bucket_name: my-storage-bucket
      repo_url: "https://github.com/GoogleCloudPlatform/cluster-toolkit.git"
      repo_ref: "00bbd75fd83579c17d43469ab48f911078841013"
      directories:
        - source_path: "examples/machine-learning"
          destination_path: "workloads/ml-examples"
          exclude:
            - ".*Dockerfile$"
            - "\\.git/.*"
```

### Synchronizing Individual Files from Git

```yaml
- group: staging
  modules:
  - id: stage-configs
    source: community/modules/file-system/gcs-sync
    settings:
      bucket_name: my-storage-bucket
      repo_url: "https://github.com/my-org/my-repo.git"
      repo_ref: "v1.2.0"
      files:
        - source_path: "configs/requirements.txt"
          destination_path: "config/requirements.txt"
        - source_path: "configs/batch-job.yaml"
          destination_path: "config/batch-job.yaml"
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |

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
| <a name="input_directories"></a> [directories](#input\_directories) | List of directories to synchronize to the GCS bucket from a remote Git repository. | <pre>list(object({<br/>    source_path      = string<br/>    destination_path = string<br/>    repo_url         = optional(string)<br/>    repo_ref         = optional(string)<br/>    exclude          = optional(list(string), [".*Dockerfile$"])<br/>  }))</pre> | `[]` | no |
| <a name="input_files"></a> [files](#input\_files) | List of individual files to synchronize to the GCS bucket from a remote Git repository. | <pre>list(object({<br/>    source_path      = string<br/>    destination_path = string<br/>    repo_url         = optional(string)<br/>    repo_ref         = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_repo_ref"></a> [repo\_ref](#input\_repo\_ref) | Default Git branch, commit SHA, or tag to checkout. It is strongly recommended to pin this to a specific commit SHA or tag rather than a mutable branch name (e.g. 'main') so triggers\_replace detects upstream updates. | `string` | `"main"` | no |
| <a name="input_repo_url"></a> [repo\_url](#input\_repo\_url) | Default Git repository URL to clone files/directories from. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_all_staged_uris"></a> [all\_staged\_uris](#output\_all\_staged\_uris) | Combined list of all destination GCS URIs managed by this module. |
| <a name="output_bucket_name"></a> [bucket\_name](#output\_bucket\_name) | Name of the target Google Cloud Storage bucket. |
| <a name="output_id"></a> [id](#output\_id) | A composite ID ensuring all staged files and directories have finished synchronizing. |
| <a name="output_staged_directory_uris"></a> [staged\_directory\_uris](#output\_staged\_directory\_uris) | List of destination GCS URIs (gs://...) for staged directories. |
| <a name="output_staged_file_uris"></a> [staged\_file\_uris](#output\_staged\_file\_uris) | List of destination GCS URIs (gs://...) for staged files. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
