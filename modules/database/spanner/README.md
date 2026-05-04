# Spanner Module
This module handles Spanner instance and database creation in Cluster Toolkit.
## Usage

```hcl
module "spanner_db" {
  source = "./modules/database/spanner"

  project_id    = var.project_id
  instance_name = "my-instance"
  databases = {
    "my-db" = {
      name = "actual-db-name"
    }
  }
}
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 5.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 5.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_project_service.spanner_api](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_service) | resource |
| [google_spanner_database.db](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/spanner_database) | resource |
| [google_spanner_database_iam_member.member](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/spanner_database_iam_member) | resource |
| [google_spanner_instance.main](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/spanner_instance) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_config"></a> [config](#input\_config) | The name of the instance's configuration. | `string` | `"regional-us-central1"` | no |
| <a name="input_databases"></a> [databases](#input\_databases) | A map of databases to create. Keys are logical names. | <pre>map(object({<br/>    name                = string<br/>    deletion_protection = optional(bool, true)<br/>    iam_members = optional(list(object({<br/>      role   = string<br/>      member = string<br/>    })), [])<br/>  }))</pre> | `{}` | no |
| <a name="input_display_name"></a> [display\_name](#input\_display\_name) | The descriptive name for this instance as it appears in UIs. | `string` | `"Spanner Instance"` | no |
| <a name="input_edition"></a> [edition](#input\_edition) | The edition of the Spanner instance (e.g., ENTERPRISE, ENTERPRISE\_PLUS, or STANDARD). | `string` | `"STANDARD"` | no |
| <a name="input_instance_name"></a> [instance\_name](#input\_instance\_name) | Name of the Spanner instance. | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to apply to the Spanner instance. | `map(string)` | `{}` | no |
| <a name="input_processing_units"></a> [processing\_units](#input\_processing\_units) | The number of processing units allocated to this instance. | `number` | `100` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID for Spanner instance. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_database_ids"></a> [database\_ids](#output\_database\_ids) | A map from logical database name to the actual created database ID. |
| <a name="output_instance_name"></a> [instance\_name](#output\_instance\_name) | The name of the Spanner instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
