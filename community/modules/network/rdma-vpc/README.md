## Description

This is an experimental VPC module.

Documentation will be updated at a later point.

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.15.0 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_vpc"></a> [vpc](#module\_vpc) | ./vpc-submodule | n/a |

## Resources

No resources.

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
| <a name="input_firewall_log_config"></a> [firewall\_log\_config](#input\_firewall\_log\_config) | Firewall log configuration for Toolkit firewall rules (var.enable\_iap\_ssh\_ingress and others) | `string` | `"DISABLE_LOGGING"` | no |
| <a name="input_firewall_rules"></a> [firewall\_rules](#input\_firewall\_rules) | List of firewall rules | `any` | `[]` | no |
| <a name="input_mtu"></a> [mtu](#input\_mtu) | The network MTU (default: 8896). Recommended values: 0 (use Compute Engine default), 1460 (default outside HPC environments), 1500 (Internet default), or 8896 (for Jumbo packets). Allowed are all values in the range 1300 to 8896, inclusively. | `number` | `8896` | no |
| <a name="input_network_address_range"></a> [network\_address\_range](#input\_network\_address\_range) | IP address range (CIDR) for global network | `string` | `"10.0.0.0/9"` | no |
| <a name="input_network_description"></a> [network\_description](#input\_network\_description) | An optional description of this resource (changes will trigger resource destroy/create) | `string` | `""` | no |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | The name of the network to be created (if unsupplied, will default to "{deployment\_name}-net") | `string` | `null` | no |
| <a name="input_network_profile"></a> [network\_profile](#input\_network\_profile) | Profile name for VPC configuration | `string` | `null` | no |
| <a name="input_network_routing_mode"></a> [network\_routing\_mode](#input\_network\_routing\_mode) | The network routing mode (default "GLOBAL") | `string` | `"GLOBAL"` | no |
| <a name="input_nic_type"></a> [nic\_type](#input\_nic\_type) | NIC type for use in modules that use the output | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The default region for Cloud resources | `string` | n/a | yes |
| <a name="input_secondary_ranges"></a> [secondary\_ranges](#input\_secondary\_ranges) | Secondary ranges that will be used in some of the subnets. Please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions. | `map(list(object({ range_name = string, ip_cidr_range = string })))` | `{}` | no |
| <a name="input_shared_vpc_host"></a> [shared\_vpc\_host](#input\_shared\_vpc\_host) | Makes this project a Shared VPC host if 'true' (default 'false') | `bool` | `false` | no |
| <a name="input_subnetworks_template"></a> [subnetworks\_template](#input\_subnetworks\_template) | Rules for creating subnetworks within the VPC | <pre>object({<br>    count          = number<br>    name_prefix    = string<br>    ip_range       = string<br>    region         = string<br>    private_access = optional(bool)<br>  })</pre> | <pre>{<br>  "count": 8,<br>  "ip_range": "192.168.0.0/16",<br>  "name_prefix": "subnet",<br>  "region": null<br>}</pre> | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_network_id"></a> [network\_id](#output\_network\_id) | ID of the new VPC network |
| <a name="output_network_name"></a> [network\_name](#output\_network\_name) | Name of the new VPC network |
| <a name="output_network_self_link"></a> [network\_self\_link](#output\_network\_self\_link) | Self link of the new VPC network |
| <a name="output_subnetwork_interfaces"></a> [subnetwork\_interfaces](#output\_subnetwork\_interfaces) | Full list of subnetwork objects belonging to the new VPC network (compatible with vm-instance) |
| <a name="output_subnetworks"></a> [subnetworks](#output\_subnetworks) | Full list of subnetwork objects belonging to the new VPC network |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
