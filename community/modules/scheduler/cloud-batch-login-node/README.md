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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_login_startup_script"></a> [login\_startup\_script](#module\_login\_startup\_script) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script | v1.0.0 |

## Resources

| Name | Type |
|------|------|
| [google_compute_instance_from_template.batch_login](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance_from_template) | resource |
| [google_compute_instance_template.batch_instance_template](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_instance_template) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_batch_job_directory"></a> [batch\_job\_directory](#input\_batch\_job\_directory) | The path of the directory on the login node in which to place the Cloud Batch job template | `string` | `"/home/batch-jobs"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, used for the job\_id | `string` | n/a | yes |
| <a name="input_gcloud_version"></a> [gcloud\_version](#input\_gcloud\_version) | The version of the gcloud cli being used. Used for output instructions. Valid inputs are `"alpha"`, `"beta"` and "" (empty string for default version) | `string` | `"alpha"` | no |
| <a name="input_instance_template"></a> [instance\_template](#input\_instance\_template) | Login VM instance template self-link | `string` | n/a | yes |
| <a name="input_job_filename"></a> [job\_filename](#input\_job\_filename) | The filename of the generated job template file | `string` | n/a | yes |
| <a name="input_job_id"></a> [job\_id](#input\_job\_id) | An id for the Cloud Batch job. Used for output instructions | `string` | n/a | yes |
| <a name="input_job_template_contents"></a> [job\_template\_contents](#input\_job\_template\_contents) | The contents of the Cloud Batch job template | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the login node. List key, value pairs | `any` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region in which to create the login node | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions for accessing the login node and submitting Cloud Batch jobs |
| <a name="output_login_node_name"></a> [login\_node\_name](#output\_login\_node\_name) | Name of the created VM |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
