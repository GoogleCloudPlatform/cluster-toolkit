<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2023 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 6.34.1 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | ~> 6.34.1 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_homefs"></a> [homefs](#module\_homefs) | ./modules/embedded/modules/file-system/filestore | n/a |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_connect_mode_file_path"></a> [connect\_mode\_file\_path](#input\_connect\_mode\_file\_path) | Toolkit deployment variable: connect\_mode\_file\_path | `string` | n/a | yes |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Toolkit deployment variable: deployment\_name | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Toolkit deployment variable: labels | `any` | n/a | yes |
| <a name="input_network_id_network0"></a> [network\_id\_network0](#input\_network\_id\_network0) | Automatically generated input from previous groups (gcluster import-inputs --help) | `any` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Toolkit deployment variable: project\_id | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Toolkit deployment variable: region | `string` | n/a | yes |
| <a name="input_subnetwork_name_network0"></a> [subnetwork\_name\_network0](#input\_subnetwork\_name\_network0) | Automatically generated input from previous groups (gcluster import-inputs --help) | `any` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | Toolkit deployment variable: zone | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->