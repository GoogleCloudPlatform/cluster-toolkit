## Description

Creates a [monitoring dashboard][gcp-dash] for the HPC cluster deployment. The
module includes a default HPC-focused dashboard with the ability to add custom
widgets as well as the option to add an empty dashboard and add widgets as
needed.

[gcp-dash]: https://cloud.google.com/monitoring/charts/predefined-dashboards

## Example

```yaml
- id: hpc_dash
  source: modules/monitoring/dashboard
  settings:
    widgets:
    - |
      {
        "text": {
          "content": "## Header",
          "format": "MARKDOWN"
        },
        "title": "Custom Text Block Widget"
      }
```

This module creates a dashboard based on the HPC dashboard (default) with an
extra text widget added as a multi-line string representing a JSON block.

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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_monitoring_dashboard.dashboard](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/monitoring_dashboard) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_base_dashboard"></a> [base\_dashboard](#input\_base\_dashboard) | Baseline dashboard template, select from HPC or Empty | `string` | `"HPC"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the monitoring dashboard instance. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_title"></a> [title](#input\_title) | Title of the created dashboard | `string` | `"Cluster Toolkit Dashboard"` | no |
| <a name="input_widgets"></a> [widgets](#input\_widgets) | List of additional widgets to add to the base dashboard. | `list(string)` | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions for accessing the monitoring dashboard |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
