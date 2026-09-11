# GCS File Module

This module creates and uploads a single file to a Google Cloud Storage (GCS) bucket, either by providing inline content as a string or by specifying a local source file path.

## Usage

### Inline Content

```yaml
- group: files
  modules:
  - id: env-file
    source: community/modules/files/gcs-file
    settings:
      bucket_name: my-storage-bucket
      object_path: config/variables.env
      content: |
        PROJECT_ID="my-project-id"
        REGION="us-central1"
```

### Local File Source

```yaml
- group: files
  modules:
  - id: script-file
    source: community/modules/files/gcs-file
    settings:
      bucket_name: my-storage-bucket
      object_path: scripts/setup.sh
      source_path: ./scripts/setup.sh
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.42 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.42 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_storage_bucket_object.file](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | Target GCS bucket name where the file will be stored. | `string` | n/a | yes |
| <a name="input_content"></a> [content](#input\_content) | Inline string content to upload to the GCS object. Mutually exclusive with source\_path. | `string` | `null` | no |
| <a name="input_content_type"></a> [content\_type](#input\_content\_type) | The Content-Type of the object (e.g., 'text/plain', 'application/json'). If omitted, it will be automatically inferred. | `string` | `null` | no |
| <a name="input_object_path"></a> [object\_path](#input\_object\_path) | GCS object path (e.g., 'config/variables.env' or 'scripts/setup.sh'). | `string` | n/a | yes |
| <a name="input_source_path"></a> [source\_path](#input\_source\_path) | Path to a local file to upload to the GCS object. Mutually exclusive with content. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_bucket"></a> [bucket](#output\_bucket) | The target GCS bucket where the object was uploaded. |
| <a name="output_crc32c"></a> [crc32c](#output\_crc32c) | The CRC32C hash of the uploaded object content. |
| <a name="output_gcs_uri"></a> [gcs\_uri](#output\_gcs\_uri) | The full GCS URI of the uploaded file. |
| <a name="output_md5hash"></a> [md5hash](#output\_md5hash) | The MD5 hash of the uploaded object content. |
| <a name="output_object_name"></a> [object\_name](#output\_object\_name) | The GCS object name of the uploaded file. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
