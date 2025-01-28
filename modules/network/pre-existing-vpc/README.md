## Description

This module discovers a VPC network that already exists in Google Cloud and
outputs network attributes that uniquely identify it for use by other modules.
The module outputs are aligned with the [vpc module][vpc] so that it can be used
as a drop-in substitute when a VPC already exists.

For example, the blueprint below discovers the "default" global network and the
"default" regional subnetwork in us-central1. With the `use` keyword, the
[vm-instance] module accepts the `network_self_link` and `subnetwork_self_link`
input variables that uniquely identify the network and subnetwork in which the
VM will be created.

[vpc]: ../vpc/README.md
[vm-instance]: ../../compute/vm-instance/README.md

### Example

```yaml
- id: network1
  source: modules/network/pre-existing-vpc
  settings:
    project_id: $(vars.project_id)
    region: us-central1

- id: example_vm
  source: modules/compute/vm-instance
  use:
  - network1
  settings:
    name_prefix: example
    machine_type: c2-standard-4
```

> **_NOTE:_** The `project_id` and `region` settings would be inferred from the
> deployment variables of the same name, but they are included here for clarity.

### Use shared-vpc

If a network is created in different project, this module can be used to
reference the network. To use a network from a different project first make sure
you have a [cloud nat][cloudnat] and [IAP][iap] forwarding. For more details,
refer [shared-vpc][shared-vpc-doc]

[cloudnat]: https://cloud.google.com/nat/docs/overview
[iap]: https://cloud.google.com/iap/docs/using-tcp-forwarding
[shared-vpc-doc]: ../../../examples/README.md#hpc-slurm-sharedvpcyaml-community-badge-experimental-badge

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
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | Name of the existing VPC network | `string` | `"default"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region in which to search for primary subnetwork | `string` | n/a | yes |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | Name of the pre-existing VPC subnetwork; defaults to var.network\_name if set to null. | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_network_id"></a> [network\_id](#output\_network\_id) | ID of the existing VPC network |
| <a name="output_network_name"></a> [network\_name](#output\_network\_name) | Name of the existing VPC network |
| <a name="output_network_self_link"></a> [network\_self\_link](#output\_network\_self\_link) | Self link of the existing VPC network |
| <a name="output_subnetwork"></a> [subnetwork](#output\_subnetwork) | Full subnetwork object in the primary region |
| <a name="output_subnetwork_address"></a> [subnetwork\_address](#output\_subnetwork\_address) | Subnetwork IP range in the primary region |
| <a name="output_subnetwork_name"></a> [subnetwork\_name](#output\_subnetwork\_name) | Name of the subnetwork in the primary region |
| <a name="output_subnetwork_self_link"></a> [subnetwork\_self\_link](#output\_subnetwork\_self\_link) | Subnetwork self-link in the primary region |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
