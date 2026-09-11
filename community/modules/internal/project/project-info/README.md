# Project Info Module

Fetches GCP project information such as project ID, project name, and project number, and exposes them as outputs.

---

## Example Usage

```yaml
  - id: project-info
    source: community/modules/internal/project/project-info
    settings:
      project_id: $(vars.project_id)
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
| [google_project.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/project) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_project_id"></a> [project\_id](#output\_project\_id) | The GCP project ID |
| <a name="output_project_name"></a> [project\_name](#output\_project\_name) | The display name of the GCP project |
| <a name="output_project_number"></a> [project\_number](#output\_project\_number) | The GCP project number retrieved from the project ID |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
