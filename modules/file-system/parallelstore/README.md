<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2024 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 5.25.0 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 5.25.0 |
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_parallelstore_instance.instance](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_parallelstore_instance) | resource |
| [null_resource.hydration](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment. | `string` | n/a | yes |
| <a name="input_import_destination_path"></a> [import\_destination\_path](#input\_import\_destination\_path) | The name of local path to import data on parallelstore instance from GCS bucket. | `string` | `null` | no |
| <a name="input_import_gcs_bucket_uri"></a> [import\_gcs\_bucket\_uri](#input\_import\_gcs\_bucket\_uri) | The name of the GCS bucket to import data from to parallelstore. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to parallel store instance. | `map(string)` | `{}` | no |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | The mount point where the contents of the device may be accessed after mounting. | `string` | `"/parallelstore"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Options describing various aspects of the parallelstore instance. | `string` | `"disable-wb-cache,thread-count=16,eq-count=8"` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of parallelstore instance. | `string` | `null` | no |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the instance is connected given in the format:<br>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#input\_private\_vpc\_connection\_peering) | The name of the VPC Network peering connection.<br>If using new VPC, please use community/modules/network/private-service-access to create private-service-access and<br>If using existing VPC with private-service-access enabled, set this manually." | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created. | `string` | n/a | yes |
| <a name="input_size_gb"></a> [size\_gb](#input\_size\_gb) | Storage size of the parallelstore instance in GB. | `number` | `12000` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Location for parallelstore instance. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions to monitor import-data operation from GCS bucket to parallelstore. |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a parallelstore instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
