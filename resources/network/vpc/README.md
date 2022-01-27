## Description
This resource creates a new VPC network along with a [cloud NAT](https://github.com/terraform-google-modules/terraform-google-cloud-nat),
[Router](https://github.com/terraform-google-modules/terraform-google-cloud-router)
and common [firewall rules](https://github.com/terraform-google-modules/terraform-google-network/tree/master/modules/firewall-rules).
This resource is based on submodules defined by the [Cloud Foundation Toolkit](https://cloud.google.com/foundation-toolkit).

The created cloud NAT (Network Address Translation) allows virtual machines
without external IP addresses create outbound connections to the internet. For
more information see the [docs](https://cloud.google.com/nat/docs/overview).

The following firewall rules are created with the VPC network:
* Allow SSH access from the Cloud Console ("35.235.240.0/20").
* Allow traffic between nodes within the VPC

### Example
```
- source: ./resources/network/vpc
  kind: terraform
  id: network1
  settings:
  - deployment_name: $(vars.deployment_name)
```
This creates a new VPC network named based on the `deployment_name` variable
with `_net` appended. `network_name` can be set manually as well as part of the
settings.

Note that `deployment_name` does not need to be set explicitly here,
it would typically be inferred from the global variable of the same name. It was
included for clarity.

## License
<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2021 Google LLC

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
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 3.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_cloud_nat"></a> [cloud\_nat](#module\_cloud\_nat) | terraform-google-modules/cloud-nat/google | ~> 1.4 |
| <a name="module_cloud_router"></a> [cloud\_router](#module\_cloud\_router) | terraform-google-modules/cloud-router/google | ~> 0.4 |
| <a name="module_firewall_rules"></a> [firewall\_rules](#module\_firewall\_rules) | terraform-google-modules/network/google//modules/firewall-rules | ~> 3.0 |
| <a name="module_vpc"></a> [vpc](#module\_vpc) | terraform-google-modules/network/google | ~> 3.0 |

## Resources

| Name | Type |
|------|------|
| [google_compute_address.nat_ips](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_address) | resource |
| [google_compute_subnetwork.primary_subnetwork](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment | `string` | n/a | yes |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | The name of the network to be created. Defaults to {deployment\_name}\_net if null. | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region where Cloud NAT and Cloud Router will be configured | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nat_ips"></a> [nat\_ips](#output\_nat\_ips) | the external IPs assigned to the NAT |
| <a name="output_network_name"></a> [network\_name](#output\_network\_name) | The name of the network created |
| <a name="output_network_self_link"></a> [network\_self\_link](#output\_network\_self\_link) | The URI of the VPC being created |
| <a name="output_subnetwork"></a> [subnetwork](#output\_subnetwork) | The subnetwork in the specified primary region |
| <a name="output_subnetwork_address"></a> [subnetwork\_address](#output\_subnetwork\_address) | The subnetwork address in the specified primary region |
| <a name="output_subnetwork_name"></a> [subnetwork\_name](#output\_subnetwork\_name) | The name of the subnetwork in the specified primary region |
| <a name="output_subnetwork_self_link"></a> [subnetwork\_self\_link](#output\_subnetwork\_self\_link) | The subnetwork self-link in the specified primary region |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
