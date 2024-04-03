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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.15.0 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_vpcs"></a> [vpcs](#module\_vpcs) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/network/vpc | v1.31.1&depth=1 |

## Resources

| Name | Type |
|------|------|
| [null_resource.vpc_validation](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_allowed_ssh_ip_ranges"></a> [allowed\_ssh\_ip\_ranges](#input\_allowed\_ssh\_ip\_ranges) | A list of CIDR IP ranges from which to allow ssh access | `list(string)` | `[]` | no |
| <a name="input_delete_default_internet_gateway_routes"></a> [delete\_default\_internet\_gateway\_routes](#input\_delete\_default\_internet\_gateway\_routes) | If set, ensure that all routes within the network specified whose names begin with 'default-route' and with a next hop of 'default-internet-gateway' are deleted | `bool` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment | `string` | n/a | yes |
| <a name="input_enable_iap_rdp_ingress"></a> [enable\_iap\_rdp\_ingress](#input\_enable\_iap\_rdp\_ingress) | Enable a firewall rule to allow Windows Remote Desktop Protocol access using IAP tunnels | `bool` | `false` | no |
| <a name="input_enable_iap_ssh_ingress"></a> [enable\_iap\_ssh\_ingress](#input\_enable\_iap\_ssh\_ingress) | Enable a firewall rule to allow SSH access using IAP tunnels | `bool` | `true` | no |
| <a name="input_enable_iap_winrm_ingress"></a> [enable\_iap\_winrm\_ingress](#input\_enable\_iap\_winrm\_ingress) | Enable a firewall rule to allow Windows Remote Management (WinRM) access using IAP tunnels | `bool` | `false` | no |
| <a name="input_enable_internal_traffic"></a> [enable\_internal\_traffic](#input\_enable\_internal\_traffic) | Enable a firewall rule to allow all internal TCP, UDP, and ICMP traffic within the network | `bool` | `true` | no |
| <a name="input_extra_iap_ports"></a> [extra\_iap\_ports](#input\_extra\_iap\_ports) | A list of TCP ports for which to create firewall rules that enable IAP for TCP forwarding (use dedicated enable\_iap variables for standard ports) | `list(string)` | `[]` | no |
| <a name="input_firewall_rules"></a> [firewall\_rules](#input\_firewall\_rules) | List of firewall rules | `any` | `[]` | no |
| <a name="input_ips_per_nat"></a> [ips\_per\_nat](#input\_ips\_per\_nat) | The number of IP addresses to allocate for each regional Cloud NAT (set to 0 to disable NAT) | `number` | `2` | no |
| <a name="input_mtu"></a> [mtu](#input\_mtu) | The network MTU (default: 8896). Recommended values: 0 (use Compute Engine default), 1460 (default outside HPC environments), 1500 (Internet default), or 8896 (for Jumbo packets). Allowed are all values in the range 1300 to 8896, inclusively. | `number` | `8896` | no |
| <a name="input_network_cidr_prefix"></a> [network\_cidr\_prefix](#input\_network\_cidr\_prefix) | The size, in CIDR prefix notation, for each network (e.g. 24 for 172.16.0.0/24); changing this will destroy every network. | `number` | `16` | no |
| <a name="input_network_count"></a> [network\_count](#input\_network\_count) | The number of vpc nettworks to create | `number` | `4` | no |
| <a name="input_network_description"></a> [network\_description](#input\_network\_description) | An optional description of this resource (changes will trigger resource destroy/create) | `string` | `""` | no |
| <a name="input_network_routing_mode"></a> [network\_routing\_mode](#input\_network\_routing\_mode) | The network dynamic routing mode | `string` | `"REGIONAL"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The default region for Cloud resources | `string` | n/a | yes |
| <a name="input_secondary_ranges"></a> [secondary\_ranges](#input\_secondary\_ranges) | Secondary ranges that will be used in some of the subnets. Please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions. | `map(list(object({ range_name = string, ip_cidr_range = string })))` | `{}` | no |
| <a name="input_super_global_ip_address_range"></a> [super\_global\_ip\_address\_range](#input\_super\_global\_ip\_address\_range) | IP address range (CIDR) that will span entire set of VPC networks | `string` | `"172.16.0.0"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_additional_networks"></a> [additional\_networks](#output\_additional\_networks) | Network interfaces for each subnetwork created by this module |
| <a name="output_network_id"></a> [network\_id](#output\_network\_id) | IDs of the new VPC network |
| <a name="output_network_names"></a> [network\_names](#output\_network\_names) | Names of the new VPC networks |
| <a name="output_network_self_links"></a> [network\_self\_links](#output\_network\_self\_links) | Self link of the new VPC network |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
