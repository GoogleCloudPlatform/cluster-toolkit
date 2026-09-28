# GCS Objects Module

This module declaratively creates and manages objects in a Google Cloud Storage (GCS) bucket from inline strings, local files, and local directories.

## Why use this module?

- **Declarative & State-Managed**: Objects created by this module are managed directly by Terraform as `google_storage_bucket_object` resources in Terraform state.
- **Drift Detection**: Any out-of-band modifications or deletions of managed objects are detected on `terraform plan`.
- **Managed Lifecycle**: When resources or deployments are destroyed via `terraform destroy`, Terraform removes all managed objects from the bucket.
- **Unified Local Ingestion**: Supports single files (inline content or local files) and entire directory trees with regex exclusions.

> [!NOTE]
> If you need to synchronize content directly from a remote Git repository at apply time, use the companion [gcs-sync](../gcs-sync/README.md) module instead.

## Usage

### Inline Content and Local Files

```yaml
- group: storage-objects
  modules:
  - id: upload-config
    source: community/modules/file-system/gcs-objects
    settings:
      bucket_name: my-storage-bucket
      files:
        - destination_path: config/variables.env
          content: |
            PROJECT_ID="my-project-id"
            REGION="us-central1"
        - destination_path: scripts/entrypoint.sh
          source_path: $(ghpc_stage("./scripts/entrypoint.sh"))
```

### Recursive Local Directory Upload

```yaml
- group: storage-objects
  modules:
  - id: upload-source
    source: community/modules/file-system/gcs-objects
    settings:
      bucket_name: my-storage-bucket
      directories:
        - destination_path: src
          source_path: $(ghpc_stage("./src"))
          exclude:
            - "\\.git/.*"
            - ".*\\.pyc$"
            - "__pycache__/.*"
```

> [!IMPORTANT]
> The `source_path` attribute resolves relative to the deployment root directory (`${path.root}`).
> In Cluster Toolkit blueprints, always wrap local paths with `$(ghpc_stage(...))` to ensure files or directories are copied to the deployment directory before Terraform execution.
>
> Patterns provided in `exclude` are matched as RE2 regular expressions against relative file paths within the directory, **not** shell globs.

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.41 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.41 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_storage_bucket_object.objects](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | Target Google Cloud Storage bucket name where objects will be stored. | `string` | n/a | yes |
| <a name="input_directories"></a> [directories](#input\_directories) | Local directories to upload recursively. Paths are resolved relative to the deployment root directory (typically staged via $(ghpc\_stage(...))). Note that patterns in 'exclude' are matched as RE2 regular expressions against the relative file path, not shell globs. | <pre>list(object({<br/>    destination_path = string<br/>    source_path      = string<br/>    exclude          = optional(list(string), [])<br/>  }))</pre> | `[]` | no |
| <a name="input_files"></a> [files](#input\_files) | Individual files to upload, from inline content or a local path. | <pre>list(object({<br/>    destination_path = string<br/>    source_path      = optional(string)<br/>    content          = optional(string)<br/>    content_type     = optional(string)<br/>  }))</pre> | `[]` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_all_staged_uris"></a> [all\_staged\_uris](#output\_all\_staged\_uris) | List of all GCS URIs for objects managed by this module. |
| <a name="output_bucket_name"></a> [bucket\_name](#output\_bucket\_name) | Target Google Cloud Storage bucket name. |
| <a name="output_gcs_uris"></a> [gcs\_uris](#output\_gcs\_uris) | Map of destination object paths to their full GCS URIs (gs://bucket/path). |
| <a name="output_objects"></a> [objects](#output\_objects) | Map of created storage bucket object resources with metadata. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
