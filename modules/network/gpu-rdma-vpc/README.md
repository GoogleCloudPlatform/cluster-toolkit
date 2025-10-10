## Description

This module accomplishes the following:

* Creates one [VPC network][cft-network]
  * Each VPC contains a variable number of subnetworks as specified in the
    `subnetworks_template` variable
  * Each subnetwork contains distinct IP address ranges
* Outputs the following unique parameters
  * `subnetwork_interfaces` which is compatible with Slurm and vm-instance
     modules
  * `subnetwork_interfaces_gke` which is compatible with GKE modules

This module is a simplified version of the VPC module and its main difference
is the variable `subnetwork_template` which is the template for all subnetworks
created within the network.  This template contains the following values:

1. `count`: The number of subnetworks to be created
1. `name_prefix`: The prefix for the subnetwork names
1. `ip_range`: [CIDR-formatted IP range][cidr]
1. `region`: The region where the subnetwork will be deployed

> [!WARNING]
> The `ip_range` should be always be large enough to split into `count`
> subnetworks and the number of required connections within.

[cft-network]: https://github.com/terraform-google-modules/terraform-google-network/tree/v10.0.0
[cidr]: https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing#CIDR_notation

### Example

This snippet uses the gpu-vpc module to create a new VPC network named
`test-rdma-net` with 8 subnetworks named `test-mrdma-sub-#` where # ranges from
0 to 7.  The subnetworks will split the `ip_range` evenly, starting from bit 16
(0 indexed).  The networks are ingested by the Slurm nodeset within the
`additional_networks` setting.

```yaml
  - id: rdma-net
    source: modules/network/gpu-rdma-vpc
    settings:
      network_name: test-rdma-net
      network_profile: https://www.googleapis.com/compute/beta/projects/$(vars.project_id)/global/networkProfiles/$(vars.zone)-vpc-roce
      network_routing_mode: REGIONAL
      subnetworks_template:
        name_prefix: test-mrdma-sub
        count: 8
        ip_range: 192.168.0.0/16
        region: $(vars.region)

  - id: a3_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network0]
    settings:
      machine_type: a3-ultragpu-8g
     additional_networks:
        $(concat(
          [{
            network=null,
            subnetwork=network1.subnetwork_self_link,
            subnetwork_project=vars.project_id,
            nic_type="GVNIC",
            queue_count=null,
            network_ip="",
            stack_type=null,
            access_config=[],
            ipv6_access_config=[],
            alias_ip_range=[]
          }],
          rdma-net.subnetwork_interfaces
        ))
      ...
```

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
| <a name="module_vpc"></a> [vpc](#module\_vpc) | terraform-google-modules/network/google | ~> 12.0 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_delete_default_internet_gateway_routes"></a> [delete\_default\_internet\_gateway\_routes](#input\_delete\_default\_internet\_gateway\_routes) | If set, ensure that all routes within the network specified whose names begin with 'default-route' and with a next hop of 'default-internet-gateway' are deleted | `bool` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment | `string` | n/a | yes |
| <a name="input_enable_internal_traffic"></a> [enable\_internal\_traffic](#input\_enable\_internal\_traffic) | DEPRECATED: enable\_internal\_traffic can not be specified for gpu-rdma-vpc. | `bool` | `null` | no |
| <a name="input_firewall_log_config"></a> [firewall\_log\_config](#input\_firewall\_log\_config) | DEPRECATED: firewall\_log\_config can not be specified for gpu-rdma-vpc. | `string` | `null` | no |
| <a name="input_firewall_rules"></a> [firewall\_rules](#input\_firewall\_rules) | DEPRECATED: firewall\_rules can not be specified for gpu-rdma-vpc. | `any` | `null` | no |
| <a name="input_mtu"></a> [mtu](#input\_mtu) | The network MTU (default: 8896). Recommended values: 0 (use Compute Engine default), 1460 (default outside HPC environments), 1500 (Internet default), or 8896 (for Jumbo packets). Allowed are all values in the range 1300 to 8896, inclusively. | `number` | `8896` | no |
| <a name="input_network_description"></a> [network\_description](#input\_network\_description) | An optional description of this resource (changes will trigger resource destroy/create) | `string` | `""` | no |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | The name of the network to be created (if unsupplied, will default to "{deployment\_name}-net") | `string` | `null` | no |
| <a name="input_network_profile"></a> [network\_profile](#input\_network\_profile) | A full or partial URL of the network profile to apply to this network.<br/>This field can be set only at resource creation time. For example, the<br/>following are valid URLs:<br/>- https://www.googleapis.com/compute/beta/projects/{projectId}/global/networkProfiles/{network_profile_name}<br/>- projects/{projectId}/global/networkProfiles/{network\_profile\_name}} | `string` | n/a | yes |
| <a name="input_network_routing_mode"></a> [network\_routing\_mode](#input\_network\_routing\_mode) | The network routing mode (default "REGIONAL") | `string` | `"REGIONAL"` | no |
| <a name="input_nic_type"></a> [nic\_type](#input\_nic\_type) | NIC type for use in modules that use the output | `string` | `"MRDMA"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The default region for Cloud resources | `string` | n/a | yes |
| <a name="input_shared_vpc_host"></a> [shared\_vpc\_host](#input\_shared\_vpc\_host) | Makes this project a Shared VPC host if 'true' (default 'false') | `bool` | `false` | no |
| <a name="input_subnetworks_template"></a> [subnetworks\_template](#input\_subnetworks\_template) | Specifications for the subnetworks that will be created within this VPC.<br/><br/>count       (number, required, number of subnets to create, default is 8)<br/>name\_prefix (string, required, subnet name prefix, default is deployment name)<br/>ip\_range    (string, required, range of IPs for all subnets to share (CIDR format), default is 192.168.0.0/16)<br/>region      (string, optional, region to deploy subnets to, defaults to vars.region) | <pre>object({<br/>    count       = number<br/>    name_prefix = string<br/>    ip_range    = string<br/>    region      = optional(string)<br/>  })</pre> | <pre>{<br/>  "count": 8,<br/>  "ip_range": "192.168.0.0/16",<br/>  "name_prefix": null,<br/>  "region": null<br/>}</pre> | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_network_id"></a> [network\_id](#output\_network\_id) | ID of the new VPC network |
| <a name="output_network_name"></a> [network\_name](#output\_network\_name) | Name of the new VPC network |
| <a name="output_network_self_link"></a> [network\_self\_link](#output\_network\_self\_link) | Self link of the new VPC network |
| <a name="output_subnetwork_interfaces"></a> [subnetwork\_interfaces](#output\_subnetwork\_interfaces) | Full list of subnetwork objects belonging to the new VPC network (compatible with vm-instance and Slurm modules) |
| <a name="output_subnetwork_interfaces_gke"></a> [subnetwork\_interfaces\_gke](#output\_subnetwork\_interfaces\_gke) | Full list of subnetwork objects belonging to the new VPC network (compatible with gke-node-pool) |
| <a name="output_subnetwork_name_prefix"></a> [subnetwork\_name\_prefix](#output\_subnetwork\_name\_prefix) | Prefix of the RDMA subnetwork names |
| <a name="output_subnetworks"></a> [subnetworks](#output\_subnetworks) | Full list of subnetwork objects belonging to the new VPC network |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
