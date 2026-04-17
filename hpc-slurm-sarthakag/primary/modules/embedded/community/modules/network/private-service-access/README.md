## Description

This module configures [private service access][psa] for the VPC specified by
the `network_id` variable. It can be used by the
[Cloud SQL Federation module][sql] or to connect [Google Cloud NetApp Volumes][gcnv].

It will automatically perform the following steps, as described in the
[Private Service Access][psa-creation] creation page:

* Create an IP Allocation with the prefix_length specified by the
  `ip_alloc_prefix_length` variable. Let Google pick the base address automatically, or specify it by using the `address` variable.
* Create a private connection that establishes a [VPC Network Peering][vpcnp]
  connection between your VPC network and the service producer's network.
* When connecting to Google Cloud NetApp Volumes, it imports and exports custom routes.

### deletion_policy
Some services like CloudSQL or NetApp Volumes delete some internal backend resources lazily. This may take up to a few hours. Deleting the PSA peering while the backend resources still exist will fail. Set `deletion_policy = "ABANDON"` to enable error-free deletion for such PSA connections. See [deletion_policy][deletion_policy].

[sql]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/database/slurm-cloudsql-federation
[psa]: https://cloud.google.com/vpc/docs/configure-private-services-access
[psa-creation]: https://cloud.google.com/vpc/docs/configure-private-services-access#procedure
[vpcnp]: https://cloud.google.com/vpc/docs/vpc-peering
[gcnv]: https://cloud.google.com/netapp/volumes/docs/discover/overview
[deletion_policy]: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_networking_connection.html#deletion_policy-1

### Example

Connecting services which use a service networking PSA connection:

```yaml
  - source: modules/network/vpc
    id: network

  # Private Service Access (PSA) requires the compute.networkAdmin role which is
  # included in the Owner role, but not Editor.
  # https://cloud.google.com/vpc/docs/configure-private-services-access#permissions
  - source: community/modules/network/private-service-access
    id: ps_connect
    use: [network]
```

Connecting [Google Cloud NetApp Volumes](https://cloud.google.com/netapp/volumes/docs/discover/overview) for using it as a shared filesystem:

```yaml
  - source: modules/network/vpc
    id: network

  - source: community/modules/network/private-service-access
    id: ps_connect
    use: [network]
    settings:
      prefix_length: 24
      service_name: "netapp.servicenetworking.goog"
      deletion_policy: "ABANDON"
```

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2025 Google LLC

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.40 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.40 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_compute_global_address.private_ip_alloc](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_global_address) | resource |
| [google_compute_network_peering_routes_config.private_vpc_peering_routes_gcnv](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_network_peering_routes_config) | resource |
| [google_service_networking_connection.private_vpc_connection](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_networking_connection) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_address"></a> [address](#input\_address) | The IP address or beginning of the address range allocated for the Private Service Access. | `string` | `null` | no |
| <a name="input_deletion_policy"></a> [deletion\_policy](#input\_deletion\_policy) | The policy to apply when deleting the Private Service Access. Leave empty or use ABANDON. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to supporting resources. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to configure Private Service Access:<br/>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_prefix_length"></a> [prefix\_length](#input\_prefix\_length) | The prefix length of the IP range allocated for the Private Service Access. | `number` | `16` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which Private Service Access will be created. | `string` | n/a | yes |
| <a name="input_service_name"></a> [service\_name](#input\_service\_name) | The name of the service to connect. Defaults to 'servicenetworking.googleapis.com'. | `string` | `"servicenetworking.googleapis.com"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_cidr_range"></a> [cidr\_range](#output\_cidr\_range) | CIDR range of the created google\_compute\_global\_address |
| <a name="output_connect_mode"></a> [connect\_mode](#output\_connect\_mode) | Services that use Private Service Access typically specify connect\_mode<br/>"PRIVATE\_SERVICE\_ACCESS". This output value sets connect\_mode and additionally<br/>blocks terraform actions until the VPC connection has been created. |
| <a name="output_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#output\_private\_vpc\_connection\_peering) | The name of the VPC Network peering connection that was created by the service provider. |
| <a name="output_reserved_ip_range"></a> [reserved\_ip\_range](#output\_reserved\_ip\_range) | Named IP range to be used by services connected with Private Service Access. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
