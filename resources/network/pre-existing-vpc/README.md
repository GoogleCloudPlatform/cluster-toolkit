## Description

This resource defines a VPC network that already exists in GCP so that it can be
used by other resources. For example, rather than creating a VPC network from
scratch for a simple deployment, the "default" network can be used from a
project. The pre-existing-vpc can be referenced in the same ways as the
[vpc resource](../vpc/README.md)

Using a pre-existing VPC created in another Resource Group can be a good way of
sharing a single network resource between resource groups.

### Example

```yaml
- source: ./resources/network/pre-existing-vpc
  kind: terraform
  id: network1
  settings:
  - project_id: $(vars.project_id)
```

This creates a pre-existing-vpc resource based on the "default" VPC network in
the GCP project. "default" is the default for network_name unless otherwise
provided. Note that the project_id setting would be inferred from the global
variable of the same name, but it was included here for clarity.

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
| [google_compute_network.vpc](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_network) | data source |
| [google_compute_subnetwork.primary_subnetwork](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | The name of the network to be created | `string` | `"default"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region where Cloud NAT and Cloud Router will be configured | `string` | n/a | yes |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | The name of the subnetwork to returned, will use network name if null. | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_network_name"></a> [network\_name](#output\_network\_name) | The name of the network created |
| <a name="output_network_self_link"></a> [network\_self\_link](#output\_network\_self\_link) | The URI of the VPC being created |
| <a name="output_subnetwork"></a> [subnetwork](#output\_subnetwork) | The subnetwork in the specified primary region |
| <a name="output_subnetwork_address"></a> [subnetwork\_address](#output\_subnetwork\_address) | The subnetwork address in the specified primary region |
| <a name="output_subnetwork_name"></a> [subnetwork\_name](#output\_subnetwork\_name) | The name of the subnetwork in the specified primary region |
| <a name="output_subnetwork_self_link"></a> [subnetwork\_self\_link](#output\_subnetwork\_self\_link) | The subnetwork self-link in the specified primary region |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
