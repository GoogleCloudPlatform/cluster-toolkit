## Description
terraform-google-sql makes it easy to create Google CloudSQL instance and implement high availability settings. This module is meant for use with Terraform 0.13+ and tested using Terraform 1.0+. The cloudsql created here is used to integrate with the slurm cluster to enable accounting data storage.

### Example
```
- source: ./resources/database/cloudsql-federation
  kind: terraform
  id: project
  settings:
    sql_instance_name: slurm-sql6-demo
    tier: "db-f1-micro"
    network: $(network1.network_name)
    nat_ips: $(network1.nat_ips)
```
This creates a new project with pre-defined project ID, a designated folder and organization and associated billing account which will be used to pay for services consumed.

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2021 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 3.0 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 3.0 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_bigquery_connection.connection](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_bigquery_connection) | resource |
| [google_sql_database.database](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_database) | resource |
| [google_sql_database_instance.instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_database_instance) | resource |
| [google_sql_user.users](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_user) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deletion_protection"></a> [deletion\_protection](#input\_deletion\_protection) | Whether or not to allow Terraform to destroy the instance. | `string` | `false` | no |
| <a name="input_nat_ips"></a> [nat\_ips](#input\_nat\_ips) | a list of NAT ips to be allow listed for the slurm cluster communication | `list(any)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region where SQL instance will be configured | `string` | n/a | yes |
| <a name="input_sql_instance_name"></a> [sql\_instance\_name](#input\_sql\_instance\_name) | name given to the sql instance for ease of identificaion | `string` | n/a | yes |
| <a name="input_tier"></a> [tier](#input\_tier) | The machine type to use for the SQL instance | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_cloudsql"></a> [cloudsql](#output\_cloudsql) | Describes a cloudsql instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->