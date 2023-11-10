# Description

This module creates the Vertex AI Notebook, to be used in tutorials.

Primarily used for FSI - MonteCarlo Tutorial: **[fsi-montecarlo-on-batch-tutorial]**.

[fsi-montecarlo-on-batch-tutorial]: ../docs/tutorials/fsi-montecarlo-on-batch/README.md

## Usage

This is a simple usage:

```yaml
  - id: bucket
    source: community/modules/file-system/cloud-storage-bucket
    settings: 
      name_prefix: my-bucket
      local_mount: /home/jupyter/my-bucket

  - id: notebook
    source: community/modules/compute/notebook
    use: [bucket]
    settings:
      name_prefix: notebook
      machine_type: n1-standard-4

```

## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_http"></a> [http](#provider\_http) | n/a |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |
| <a name="provider_template"></a> [template](#provider\_template) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_notebooks_instance.instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/notebooks_instance) | resource |
| [google_storage_bucket_object.get_iteration_sh](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.get_mc_reqs](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.ipynb_obj_fsi](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.mc_obj_yaml](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.mc_run](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.mount_script](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
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
| <a name="input_create_tutorial"></a> [create\_tutorial](#input\_create\_tutorial) | Which tutorial files should be created. | `string` | `""` | no |
| <a name="input_dataset_id"></a> [dataset\_id](#input\_dataset\_id) | Bigquery dataset id | `string` | n/a | yes |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment; used as part of name of the GCS bucket. | `string` | n/a | yes |
| <a name="input_gcs_bucket_path"></a> [gcs\_bucket\_path](#input\_gcs\_bucket\_path) | Bucket name | `string` | `null` | no |
| <a name="input_image_family"></a> [image\_family](#input\_image\_family) | Image Family | `string` | `""` | no |
| <a name="input_image_project_id"></a> [image\_project\_id](#input\_image\_project\_id) | Project Id for Image hosting | `string` | `""` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the resource Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | The machine type to employ | `string` | n/a | yes |
| <a name="input_mount_runner"></a> [mount\_runner](#input\_mount\_runner) | mount content | `any` | n/a | yes |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Name Prefix. | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which GCS bucket will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region to run project | `string` | n/a | yes |
| <a name="input_table_id"></a> [table\_id](#input\_table\_id) | Bigquery table id | `string` | n/a | yes |
| <a name="input_topic_id"></a> [topic\_id](#input\_topic\_id) | Pubsub Topic Name | `string` | n/a | yes |
| <a name="input_topic_schema"></a> [topic\_schema](#input\_topic\_schema) | Pubsub Topic schema | `string` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | The zone to deploy to | `string` | n/a | yes |

## Outputs

No outputs.
