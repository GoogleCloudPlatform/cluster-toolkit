## Description

This module discovers a subnetwork that already exists in Google Cloud and
outputs subnetwork attributes that uniquely identify it for use by other modules.

For example, the blueprint below discovers the referred to subnetwork.
With the `use` keyword, the [vm-instance] module accepts the `subnetwork_self_link`
input variables that uniquely identify the subnetwork in which the VM will be created.

[vpc]: ../vpc/README.md
[vm-instance]: ../../compute/vm-instance/README.md

> **_NOTE:_** Additional IAM work is needed for this to work correctly.

### Example

```yaml
- id: network
  source: modules/network/pre-existing-subnetwork
  settings:
    subnetwork_self_link: https://www.googleapis.com/compute/v1/projects/name-of-host-project/regions/REGION/subnetworks/SUBNETNAME

- id: example_vm
  source: modules/compute/vm-instance
  use:
  - network
  settings:
    name_prefix: example
    machine_type: c2-standard-4
```

As described in documentation:
[https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork]

If subnetwork_self_link is provided then name,region,project is ignored.

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2026 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5 |
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
| [google_compute_subnetwork.primary_subnetwork](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_project"></a> [project](#input\_project) | Name of the project that owns the subnetwork | `string` | `null` | no |
| <a name="input_region"></a> [region](#input\_region) | Region in which to search for primary subnetwork | `string` | `null` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | Name of the pre-existing VPC subnetwork | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Self-link of the subnet in the VPC | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_subnetwork"></a> [subnetwork](#output\_subnetwork) | Full subnetwork object in the primary region |
| <a name="output_subnetwork_address"></a> [subnetwork\_address](#output\_subnetwork\_address) | Subnetwork IP range in the primary region |
| <a name="output_subnetwork_name"></a> [subnetwork\_name](#output\_subnetwork\_name) | Name of the subnetwork in the primary region |
| <a name="output_subnetwork_self_link"></a> [subnetwork\_self\_link](#output\_subnetwork\_self\_link) | Subnetwork self-link in the primary region |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
