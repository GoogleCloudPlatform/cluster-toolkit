## Description

This module creates a new VPC network along with a
[cloud NAT](https://github.com/terraform-google-modules/terraform-google-cloud-nat),
[Router](https://github.com/terraform-google-modules/terraform-google-cloud-router)
and common [firewall rules](https://github.com/terraform-google-modules/terraform-google-network/tree/master/modules/firewall-rules).
This module is based on submodules defined by the
[Cloud Foundation Toolkit](https://cloud.google.com/foundation-toolkit).

The created cloud NAT (Network Address Translation) allows virtual machines
without external IP addresses to create outbound connections to the internet.
For more information see the [docs](https://cloud.google.com/nat/docs/overview).

The following firewall rules are created with the VPC network:

* Allow SSH access from the Cloud Console ("35.235.240.0/20").
* Allow traffic between nodes within the VPC

Additionally, [Google Private Access][gpa] is enabled by default on all
subnetworks unless it is explicitly disabled. This setting ensures that all VMs
can use Google services such as [Cloud Storage][gcs] even if they do not have
public IP addresses or Cloud NAT is disabled.

[gpa]: https://cloud.google.com/vpc/docs/private-google-access
[gcs]: https://cloud.google.com/storage

### Primary and additional subnetworks

This module will, at minimum, provision a "primary" subnetwork in which most
resources are expected to be provisioned. These are controlled by the following
input variables:

* `var.subnetwork_name` and `var.subnetwork_size`
* `var.primary_subnetwork`
* `var.additional_subnetworks`

Both `var.primary_subnetwork` and `var.additional_subnetworks` behave
identically to the [Cloud Foundation Toolkit subnets module][cftsubnets] with
the lone exception that the IP range for each subnet is constructed
automatically by calculating the most compact set of subnetworks. The size of
each individual subnetwork is specified with the `new_bits` key and the base of
the global VPC network is specified using `var.network_address_range`.

[cftsubnets]: https://github.com/terraform-google-modules/terraform-google-network/tree/master/modules/subnets

If explicitly supplied, `var.primary_subnetwork` defines all properties of the
primary subnetwork. If `var.primary_subnetwork` is left at its default value of
`null`, then a default primary subnetwork will be constructed from
`var.subnetwork_name` and `var.subnetwork_size`. If no value is supplied for
`var.subnetwork_name`, a default value is constructed from
`var.deployment_name`.

Additional subnetworks are optionally supplied explicitly with
`var.additional_subnetworks`.

### Example

```yaml
- source: modules/network/vpc
  kind: terraform
  id: network1
  settings:
  - deployment_name: $(vars.deployment_name)
```

This creates a new VPC network named based on the `deployment_name` variable
with `_net` appended. `network_name` can be set manually as well as part of the
settings.

> **_NOTE:_** `deployment_name` does not need to be set explicitly here. It
> would typically be inferred from the deployment variable of the same name. It
> is included here for clarity.

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

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_cloud_router"></a> [cloud\_router](#module\_cloud\_router) | terraform-google-modules/cloud-router/google | ~> 1.3 |
| <a name="module_firewall_rules"></a> [firewall\_rules](#module\_firewall\_rules) | terraform-google-modules/network/google//modules/firewall-rules | ~> 5.0 |
| <a name="module_nat_ip_addresses"></a> [nat\_ip\_addresses](#module\_nat\_ip\_addresses) | terraform-google-modules/address/google | ~> 3.1 |
| <a name="module_vpc"></a> [vpc](#module\_vpc) | terraform-google-modules/network/google | ~> 5.0 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_additional_subnetworks"></a> [additional\_subnetworks](#input\_additional\_subnetworks) | List of additional subnetworks in which to create resources.<br><br>  subnet\_name           (string, required, Name of subnet; will be replaced by var.subnetwork\_name or its default value)<br>  subnet\_region         (string, required, will be replaced by var.region)<br>  new\_bits              (number, required, Additional CIDR bits to determine subnetwork size)<br>  subnet\_private\_access (bool, optional, Enable Private Access on subnetwork)<br>  subnet\_flow\_logs      (map(string), optional, Configure Flow Logs see terraform-google-network module)<br>  description           (string, optional, Description of Network)<br>  purpose               (string, optional, related to Load Balancing)<br>  role                  (string, optional, related to Load Balancing) | `list(map(string))` | `[]` | no |
| <a name="input_delete_default_internet_gateway_routes"></a> [delete\_default\_internet\_gateway\_routes](#input\_delete\_default\_internet\_gateway\_routes) | If set, ensure that all routes within the network specified whose names begin with 'default-route' and with a next hop of 'default-internet-gateway' are deleted | `bool` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment | `string` | n/a | yes |
| <a name="input_ips_per_nat"></a> [ips\_per\_nat](#input\_ips\_per\_nat) | The number of IP addresses to allocate for each regional Cloud NAT (set to 0 to disable NAT) | `number` | `2` | no |
| <a name="input_mtu"></a> [mtu](#input\_mtu) | The network MTU (If set to 0, meaning MTU is unset - defaults to '1460'). Recommended values: 1460 (default for historic reasons), 1500 (Internet default), or 8896 (for Jumbo packets). Allowed are all values in the range 1300 to 8896, inclusively. | `number` | `0` | no |
| <a name="input_network_address_range"></a> [network\_address\_range](#input\_network\_address\_range) | IP address range (CIDR) for global network | `string` | `"10.0.0.0/9"` | no |
| <a name="input_network_description"></a> [network\_description](#input\_network\_description) | An optional description of this resource (changes will trigger resource destroy/create) | `string` | `""` | no |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | The name of the network to be created (if unsupplied, will default to "{deployment\_name}-net") | `string` | `null` | no |
| <a name="input_network_routing_mode"></a> [network\_routing\_mode](#input\_network\_routing\_mode) | The network routing mode (default "GLOBAL") | `string` | `"GLOBAL"` | no |
| <a name="input_primary_subnetwork"></a> [primary\_subnetwork](#input\_primary\_subnetwork) | Primary (default) subnetwork in which to create resources. If null, a default value will be constructed.<br><br>  subnet\_name           (string, required, Name of subnet; will be replaced by var.subnetwork\_name or its default value)<br>  subnet\_region         (string, required, will be replaced by var.region)<br>  new\_bits              (number, optional, Additional CIDR bits to determine subnetwork size; will default to var.subnetwork\_size)<br>  subnet\_private\_access (bool, optional, Enable Private Access on subnetwork)<br>  subnet\_flow\_logs      (map(string), optional, Configure Flow Logs see terraform-google-network module)<br>  description           (string, optional, Description of Network)<br>  purpose               (string, optional, related to Load Balancing)<br>  role                  (string, optional, related to Load Balancing) | `map(string)` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The default region for Cloud resources | `string` | n/a | yes |
| <a name="input_shared_vpc_host"></a> [shared\_vpc\_host](#input\_shared\_vpc\_host) | Makes this project a Shared VPC host if 'true' (default 'false') | `bool` | `false` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | The name of the network to be created (if unsupplied, will default to "{deployment\_name}-primary-subnet") | `string` | `null` | no |
| <a name="input_subnetwork_size"></a> [subnetwork\_size](#input\_subnetwork\_size) | The size, in CIDR bits, of the primary subnetwork unless explicitly supplied in var.primary\_subnetwork | `number` | `15` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nat_ips"></a> [nat\_ips](#output\_nat\_ips) | the external IPs assigned to the NAT |
| <a name="output_network_name"></a> [network\_name](#output\_network\_name) | The name of the network created |
| <a name="output_network_self_link"></a> [network\_self\_link](#output\_network\_self\_link) | The URI of the VPC being created |
| <a name="output_subnetwork"></a> [subnetwork](#output\_subnetwork) | The primary subnetwork object created by the input variable primary\_subnetwork |
| <a name="output_subnetwork_address"></a> [subnetwork\_address](#output\_subnetwork\_address) | The address range of the primary subnetwork |
| <a name="output_subnetwork_name"></a> [subnetwork\_name](#output\_subnetwork\_name) | The name of the primary subnetwork |
| <a name="output_subnetwork_self_link"></a> [subnetwork\_self\_link](#output\_subnetwork\_self\_link) | The self-link to the primary subnetwork |
| <a name="output_subnetworks"></a> [subnetworks](#output\_subnetworks) | All subnetwork resources created by this module |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
