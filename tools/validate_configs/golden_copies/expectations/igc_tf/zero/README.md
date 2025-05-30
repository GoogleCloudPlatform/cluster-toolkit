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
| <a name="module_network0"></a> [network0](#module\_network0) | ./modules/embedded/modules/network/vpc | n/a |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Toolkit deployment variable: deployment\_name | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Toolkit deployment variable: labels | `any` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Toolkit deployment variable: project\_id | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Toolkit deployment variable: region | `string` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | Toolkit deployment variable: zone | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nat_ips_network0"></a> [nat\_ips\_network0](#output\_nat\_ips\_network0) | Generated output from module 'network0' |
| <a name="output_network_id_network0"></a> [network\_id\_network0](#output\_network\_id\_network0) | Automatically-generated output exported for use by later deployment groups |
| <a name="output_subnetwork_name_network0"></a> [subnetwork\_name\_network0](#output\_subnetwork\_name\_network0) | Generated output from module 'network0' |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->