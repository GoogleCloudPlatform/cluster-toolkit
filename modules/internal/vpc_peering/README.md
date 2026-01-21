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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_compute_network_peering.peering](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_network_peering) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_export_custom_routes"></a> [export\_custom\_routes](#input\_export\_custom\_routes) | (Optional) Whether to export the custom routes to the peer network. Defaults to false. | `bool` | `null` | no |
| <a name="input_import_custom_routes"></a> [import\_custom\_routes](#input\_import\_custom\_routes) | (Optional) Whether to import the custom routes from the peer network. Defaults to false. | `bool` | `null` | no |
| <a name="input_import_subnet_routes_with_public_ip"></a> [import\_subnet\_routes\_with\_public\_ip](#input\_import\_subnet\_routes\_with\_public\_ip) | (Optional) Whether subnet routes with public IP range are imported. | `bool` | `null` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the peering. | `string` | n/a | yes |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The primary network of the peering. | `string` | n/a | yes |
| <a name="input_peer_network_self_link"></a> [peer\_network\_self\_link](#input\_peer\_network\_self\_link) | The peer network in the peering. The peer network may belong to a different project. | `string` | n/a | yes |
| <a name="input_stack_type"></a> [stack\_type](#input\_stack\_type) | (Optional) Which IP version(s) of traffic and routes are allowed to be imported or exported between peer networks. | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_peering_name"></a> [peering\_name](#output\_peering\_name) | Name of the peering. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
