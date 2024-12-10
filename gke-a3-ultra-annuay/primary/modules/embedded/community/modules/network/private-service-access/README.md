## Description

This module configures [private service access][psa] for the VPC specified by
the `network_id` variable. It can be used by the
[Cloud SQL Federation module][sql].

It will automatically perform the following steps, as described in the
[Private Service Access][psa-creation] creation page:

* Create an IP Allocation with the prefix_length specified by the
  `ip_alloc_prefix_length` variable.
* Create a private connection that establishes a [VPC Network Peering][vpcnp]
  connection between your VPC network and the service producer's network

[sql]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/database/slurm-cloudsql-federation
[psa]: https://cloud.google.com/vpc/docs/configure-private-services-access
[psa-creation]: https://cloud.google.com/vpc/docs/configure-private-services-access#procedure
[vpcnp]: https://cloud.google.com/vpc/docs/vpc-peering

### Example

```yaml
  - source: modules/network/vpc
    id: network

  - source: community/modules/network/private-service-access
    id: ps_connect
    use: [network]
```

## License

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 3.83 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 3.83 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_compute_global_address.private_ip_alloc](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_global_address) | resource |
| [google_service_networking_connection.private_vpc_connection](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_networking_connection) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_address"></a> [address](#input\_address) | The IP address or beginning of the address range allocated for the private service access. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to supporting resources. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to configure private service Access.:<br/>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_prefix_length"></a> [prefix\_length](#input\_prefix\_length) | The prefix length of the IP range allocated for the private service access. | `number` | `16` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which Private Service Access will be created. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_cidr_range"></a> [cidr\_range](#output\_cidr\_range) | CIDR range of the created google\_compute\_global\_address |
| <a name="output_connect_mode"></a> [connect\_mode](#output\_connect\_mode) | Services that use Private Service Access typically specify connect\_mode<br/>"PRIVATE\_SERVICE\_ACCESS". This output value sets connect\_mode and additionally<br/>blocks terraform actions until the VPC connection has been created. |
| <a name="output_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#output\_private\_vpc\_connection\_peering) | The name of the VPC Network peering connection that was created by the service provider. |
| <a name="output_reserved_ip_range"></a> [reserved\_ip\_range](#output\_reserved\_ip\_range) | Named IP range to be used by services connected with Private Service Access. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
