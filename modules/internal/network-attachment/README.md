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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.15.0 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 6.0.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 6.0.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_compute_network_attachment.self](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_network_attachment) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_connection_preference"></a> [connection\_preference](#input\_connection\_preference) | The connection preference of service attachment. | `string` | `"ACCEPT_AUTOMATIC"` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the resource. Provided by the client when the resource is created | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The ID of the project in which the resource belongs. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region where the network attachment resides | `string` | n/a | yes |
| <a name="input_subnetwork_self_links"></a> [subnetwork\_self\_links](#input\_subnetwork\_self\_links) | An array of selfLinks of subnets to use for endpoints in the producers that connect to this network attachment. | `list(string)` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_self_link"></a> [self\_link](#output\_self\_link) | Server-defined URL for the resource. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
