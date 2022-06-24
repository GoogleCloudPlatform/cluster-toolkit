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
| <a name="requirement_local"></a> [local](#requirement\_local) | >= 2.0.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_local"></a> [local](#provider\_local) | >= 2.0.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [local_file.job_template](https://registry.terraform.io/providers/hashicorp/local/latest/docs/resources/file) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_gcloud_version"></a> [gcloud\_version](#input\_gcloud\_version) | The version of the gcloud cli being used. Used for output instructions. | `string` | `"alpha"` | no |
| <a name="input_instance_template"></a> [instance\_template](#input\_instance\_template) | Compute VM instance template self-link to be used for Batch compute node. | `string` | `null` | no |
| <a name="input_job_id"></a> [job\_id](#input\_job\_id) | An id for the batch job. Used for output instructions and file naming. | `string` | `"my_job"` | no |
| <a name="input_log_policy"></a> [log\_policy](#input\_log\_policy) | Create a block to define log policy.<br>When set to `CLOUD_LOGGING`, logs will be sent to Cloud Logging.<br>When set to `PATH`, path must be added to generated template.<br>When set to `DESTINATION_UNSPECIFIED`, logs will not be preserved. | `string` | `"CLOUD_LOGGING"` | no |
| <a name="input_region"></a> [region](#input\_region) | The region in which to run the Cloud Batch job | `string` | n/a | yes |
| <a name="input_runnable"></a> [runnable](#input\_runnable) | A string to be executed as the main workload of the Batch job. This will be used to populate the generated template. | `string` | `"## Add your workload here"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions for submitting Cloud Batch job. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
