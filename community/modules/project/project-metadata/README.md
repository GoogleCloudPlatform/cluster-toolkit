# Project Metadata Module

Sets one or more Google Cloud Compute Engine project metadata items (`google_compute_project_metadata_item`). Accepts either a single `key` and `value` pair or a map of `metadata` dictionary items.

## Example

```yaml
- group: project-metadata
  modules:
  - id: set-metadata
    source: community/modules/project/project-metadata
    settings:
      project_id: my-project-id
      key: custom_metadata_key
      value: custom_metadata_value
      # OR using a map:
      # metadata:
      #   KEY_1: VALUE_1
      #   KEY_2: VALUE_2
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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.0.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_project_metadata_item.items](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_project_metadata_item) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_key"></a> [key](#input\_key) | Optional single metadata key (if not using the metadata map). | `string` | `""` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | A map of key/value pairs to set as project compute metadata items. | `map(string)` | `{}` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID where compute metadata will be set. | `string` | n/a | yes |
| <a name="input_value"></a> [value](#input\_value) | Optional single metadata value (if not using the metadata map). | `string` | `""` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_metadata_items"></a> [metadata\_items](#output\_metadata\_items) | A map of created or updated project metadata keys and values. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
