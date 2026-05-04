## Description

This module runs Spanner DDL migrations from a specified directory using `gcloud`.
It looks for `.up.sql` files in `${var.migrations_dir}/${var.sub_directory}/` and applies them in alphabetical order.

### Software Requirements

* `gcloud` CLI

### Example

```yaml
  - id: run_migrations
    source: community/modules/scripts/spanner-migrations-runner
    settings:
      project_id: $(vars.project_id)
      instance_name: my-spanner-instance
      database_name: my-database
      migrations_dir: $(ghpc_stage("my-assets/migrations"))
      sub_directory: db1
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_null"></a> [null](#requirement\_null) | >= 3.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_null"></a> [null](#provider\_null) | >= 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [null_resource.run_migrations](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_database_name"></a> [database\_name](#input\_database\_name) | The Spanner database name. | `string` | n/a | yes |
| <a name="input_instance_name"></a> [instance\_name](#input\_instance\_name) | The Spanner instance name. | `string` | n/a | yes |
| <a name="input_migrations_dir"></a> [migrations\_dir](#input\_migrations\_dir) | The migrations directory. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to deploy to. | `string` | n/a | yes |
| <a name="input_proto_descriptors_file"></a> [proto\_descriptors\_file](#input\_proto\_descriptors\_file) | Optional path to a compiled proto descriptors file (.pb) needed for custom types in migrations. | `string` | `null` | no |
| <a name="input_sub_directory"></a> [sub\_directory](#input\_sub\_directory) | Optional sub-directory within migrations\_dir to search for SQL files. | `string` | `""` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_migration_completion_id"></a> [migration\_completion\_id](#output\_migration\_completion\_id) | The ID of the migration completion anchor. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
