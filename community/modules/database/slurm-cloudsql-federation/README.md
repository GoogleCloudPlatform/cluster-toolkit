## Description

terraform-google-sql makes it easy to create a Google CloudSQL instance and
implement high availability settings. This module is meant for use with
Terraform 0.13+ and tested using Terraform 1.0+.

The cloudsql created here is used to integrate with the slurm cluster to enable
accounting data storage.

### Example

```yaml
- id: project
  source: community/modules/database/cloudsql-federation
  use: [network1]
  settings:
    sql_instance_name: slurm-sql6-demo
    tier: "db-f1-micro"
```

This creates a cloud sql instance, including a database, user that would allow
the slurm cluster to use as an external DB. In addition, it will allow BigQuery
to run federated query through it.

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2022 Google LLC

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 3.83 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 3.83 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_compute_global_address.private_ip_address](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_global_address) | resource |
| [google-beta_google_service_networking_connection.private_vpc_connection](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_service_networking_connection) | resource |
| [google_bigquery_connection.connection](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/bigquery_connection) | resource |
| [google_sql_database.database](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_database) | resource |
| [google_sql_database_instance.instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_database_instance) | resource |
| [google_sql_user.users](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_user) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [random_password.password](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/password) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deletion_protection"></a> [deletion\_protection](#input\_deletion\_protection) | Whether or not to allow Terraform to destroy the instance. | `string` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the instance is going to be created in.:<br>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region where SQL instance will be configured | `string` | n/a | yes |
| <a name="input_sql_instance_name"></a> [sql\_instance\_name](#input\_sql\_instance\_name) | name given to the sql instance for ease of identificaion | `string` | n/a | yes |
| <a name="input_sql_password"></a> [sql\_password](#input\_sql\_password) | Password for the SQL database. | `any` | `null` | no |
| <a name="input_sql_username"></a> [sql\_username](#input\_sql\_username) | Username for the SQL database | `string` | `"slurm"` | no |
| <a name="input_tier"></a> [tier](#input\_tier) | The machine type to use for the SQL instance | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_cloudsql"></a> [cloudsql](#output\_cloudsql) | Describes the cloudsql instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
