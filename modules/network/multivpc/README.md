## Description

This module accomplishes the following:

* Creates 2 to 8 [VPC networks][vpc]
  * Each VPC contains exactly 1 subnetwork
  * Each subnetwork contains distinct IP address ranges
* Outputs the `additional_networks` parameter, which is compatible with Slurm
  modules

There are 4 variables that differentiate this module from the standard VPC
module.

1. `network_prefix`: The name prefix of the VPCs to be created.  All
   networks and subnetworks will start with this and end with a unique number.
1. `network_count`: The number of VPCs to be created.
1. `global_ip_address_range`: [CIDR-formatted IP range][cidr]
1. `network_cidr_suffix`: The CIDR suffix that defines the address
   space that the individual VPCs will cover.

> [!WARNING]
> The `network_cidr_suffix` should be always be larger than the CIDR suffix on
> `global_ip_address_range`.  The difference between these two suffixes should
> be large enough to accommodate the number of VPCs that are being deployed
> (e.g. CIDR suffix bit difference <= `ceil(log2(network_count)))`).
<!-- MD028/no-blanks-blockquote -->
> [!NOTE]
> For deployments that need multiple VPCs that do not meet this use-case, users
> should deploy multiple individual VPC modules.

[vpc]: ../vpc/README.md
[cidr]: https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing#CIDR_notation

### Example

This snippet uses the multivpc module to create 8 new VPC networks named
`multivpc-net-#` where # ranges from 0 to 7. Additionally, it creates 1
subnetwork in each VPC.

```yaml
  - id: network
    source: modules/network/vpc

  - id: multinetwork
    source: modules/network/multivpc
    settings:
      network_name_prefix: multivpc-net
      network_count: 8
      global_ip_address_range: 172.16.0.0/12
      subnetwork_cidr_suffix: 16

  - id: a3_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network, multinetwork]
    settings:
      machine_type: a3-highgpu-8g
      ...
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.4.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_vpcs"></a> [vpcs](#module\_vpcs) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/network/vpc | v1.35.0&depth=1 |

## Resources

| Name | Type |
|------|------|
| [terraform_data.global_ip_cidr_suffix](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

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
| <a name="input_global_ip_address_range"></a> [global\_ip\_address\_range](#input\_global\_ip\_address\_range) | IP address range (CIDR) that will span entire set of VPC networks | `string` | `"172.16.0.0/12"` | no |
| <a name="input_ips_per_nat"></a> [ips\_per\_nat](#input\_ips\_per\_nat) | The number of IP addresses to allocate for each regional Cloud NAT (set to 0 to disable NAT) | `number` | `2` | no |
| <a name="input_mtu"></a> [mtu](#input\_mtu) | The network MTU (default: 8896). Recommended values: 0 (use Compute Engine default), 1460 (default outside HPC environments), 1500 (Internet default), or 8896 (for Jumbo packets). Allowed are all values in the range 1300 to 8896, inclusively. | `number` | `8896` | no |
| <a name="input_network_count"></a> [network\_count](#input\_network\_count) | The number of vpc nettworks to create | `number` | `4` | no |
| <a name="input_network_description"></a> [network\_description](#input\_network\_description) | An optional description of this resource (changes will trigger resource destroy/create) | `string` | `""` | no |
| <a name="input_network_interface_defaults"></a> [network\_interface\_defaults](#input\_network\_interface\_defaults) | The template of the network settings to be used on all vpcs. | <pre>object({<br>    network            = optional(string)<br>    subnetwork         = optional(string)<br>    subnetwork_project = optional(string)<br>    network_ip         = optional(string, "")<br>    nic_type           = optional(string, "GVNIC")<br>    stack_type         = optional(string, "IPV4_ONLY")<br>    queue_count        = optional(string)<br>    access_config = optional(list(object({<br>      nat_ip                 = string<br>      network_tier           = string<br>      public_ptr_domain_name = string<br>    })), [])<br>    ipv6_access_config = optional(list(object({<br>      network_tier           = string<br>      public_ptr_domain_name = string<br>    })), [])<br>    alias_ip_range = optional(list(object({<br>      ip_cidr_range         = string<br>      subnetwork_range_name = string<br>    })), [])<br>  })</pre> | <pre>{<br>  "access_config": [],<br>  "alias_ip_range": [],<br>  "ipv6_access_config": [],<br>  "network": null,<br>  "network_ip": "",<br>  "nic_type": "GVNIC",<br>  "queue_count": null,<br>  "stack_type": "IPV4_ONLY",<br>  "subnetwork": null,<br>  "subnetwork_project": null<br>}</pre> | no |
| <a name="input_network_name_prefix"></a> [network\_name\_prefix](#input\_network\_name\_prefix) | The base name of the vpcs and their subnets, will be appended with a sequence number | `string` | `""` | no |
| <a name="input_network_routing_mode"></a> [network\_routing\_mode](#input\_network\_routing\_mode) | The network dynamic routing mode | `string` | `"REGIONAL"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The default region for Cloud resources | `string` | n/a | yes |
| <a name="input_subnetwork_cidr_suffix"></a> [subnetwork\_cidr\_suffix](#input\_subnetwork\_cidr\_suffix) | The size, in CIDR suffix notation, for each network (e.g. 24 for 172.16.0.0/24); changing this will destroy every network. | `number` | `16` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_additional_networks"></a> [additional\_networks](#output\_additional\_networks) | Network interfaces for each subnetwork created by this module |
| <a name="output_network_ids"></a> [network\_ids](#output\_network\_ids) | IDs of the new VPC network |
| <a name="output_network_names"></a> [network\_names](#output\_network\_names) | Names of the new VPC networks |
| <a name="output_network_self_links"></a> [network\_self\_links](#output\_network\_self\_links) | Self link of the new VPC network |
| <a name="output_subnetwork_addresses"></a> [subnetwork\_addresses](#output\_subnetwork\_addresses) | IP address range of the primary subnetwork |
| <a name="output_subnetwork_names"></a> [subnetwork\_names](#output\_subnetwork\_names) | Names of the subnetwork created in each network |
| <a name="output_subnetwork_self_links"></a> [subnetwork\_self\_links](#output\_subnetwork\_self\_links) | Self link of the primary subnetwork |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
