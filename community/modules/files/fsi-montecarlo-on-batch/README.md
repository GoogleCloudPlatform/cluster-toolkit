## Description

Copy files to a target GCS bucket.

Primarily used for FSI - MonteCarlo Tutorial **[fsi-montecarlo-on-batch-tutorial]**.

[fsi-montecarlo-on-batch-tutorial]: ../docs/tutorials/fsi-montecarlo-on-batch/README.md

## Requirements

No requirements.

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | n/a |
| <a name="provider_http"></a> [http](#provider\_http) | n/a |
| <a name="provider_random"></a> [random](#provider\_random) | n/a |
| <a name="provider_template"></a> [template](#provider\_template) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.get_iteration_sh](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.get_mc_reqs](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.get_requirements](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.ipynb_obj_fsi](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.mc_obj_yaml](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.mc_run](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.run_batch_py](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [http_http.batch_py](https://registry.terraform.io/providers/hashicorp/http/latest/docs/data-sources/http) | data source |
| [http_http.batch_requirements](https://registry.terraform.io/providers/hashicorp/http/latest/docs/data-sources/http) | data source |
| [template_file.ipynb_fsi](https://registry.terraform.io/providers/hashicorp/template/latest/docs/data-sources/file) | data source |
| [template_file.mc_run_py](https://registry.terraform.io/providers/hashicorp/template/latest/docs/data-sources/file) | data source |
| [template_file.mc_run_yaml](https://registry.terraform.io/providers/hashicorp/template/latest/docs/data-sources/file) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_dataset_id"></a> [dataset\_id](#input\_dataset\_id) | Bigquery dataset id | `string` | n/a | yes |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment; used as part of name of the GCS bucket. | `string` | n/a | yes |
| <a name="input_gcs_bucket_path"></a> [gcs\_bucket\_path](#input\_gcs\_bucket\_path) | Bucket name | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which GCS bucket will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region to run project | `string` | n/a | yes |
| <a name="input_table_id"></a> [table\_id](#input\_table\_id) | Bigquery table id | `string` | n/a | yes |
| <a name="input_topic_id"></a> [topic\_id](#input\_topic\_id) | Pubsub Topic Name | `string` | n/a | yes |
| <a name="input_topic_schema"></a> [topic\_schema](#input\_topic\_schema) | Pubsub Topic schema | `string` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | The zone to deploy to | `string` | n/a | yes |

## Outputs

No outputs.
