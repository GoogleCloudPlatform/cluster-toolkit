## Description

Creates a BigQuery table with a specified schema.

Primarily used for FSI - MonteCarlo Tutorial: **[fsi-montecarlo-on-batch-tutorial]**.

[fsi-montecarlo-on-batch-tutorial]: ../docs/tutorials/fsi-montecarlo-on-batch/README.md

## Usage

```yaml
id: bq-table
    source: community/modules/database/bigquery_table
    use: [bq-dataset]
    settings:
      table_id: null
      table_schema: 'json schema'
```

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
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_bigquery_table.pbsb](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/bigquery_table) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_dataset_id"></a> [dataset\_id](#input\_dataset\_id) | Table name to be used to create the new BQ Table | `string` | n/a | yes |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region where SQL instance will be configured | `string` | n/a | yes |
| <a name="input_table_id"></a> [table\_id](#input\_table\_id) | Table name to be used to create the new BQ Table | `string` | n/a | yes |
| <a name="input_table_schema"></a> [table\_schema](#input\_table\_schema) | Schema used to create the new BQ Table | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_dataset_id"></a> [dataset\_id](#output\_dataset\_id) | ID of BQ dataset |
| <a name="output_table_id"></a> [table\_id](#output\_table\_id) | ID of created BQ table |
| <a name="output_table_name"></a> [table\_name](#output\_table\_name) | Name of created BQ table |
