# Module: Slurm Instance

<!-- mdformat-toc start --slug=github --no-anchors --maxlevel=6 --minlevel=1 -->

- [Module: Slurm Instance](#module-slurm-instance)
  - [Overview](#overview)
  - [Module API](#module-api)

<!-- mdformat-toc end -->

## Overview

This module creates a [compute instance](../../../../docs/glossary.md#vm) from
[instance template](../../../../docs/glossary.md#instance-template) for a
[Slurm cluster](../slurm_cluster/README.md).

> **NOTE:** This module is only intended to be used by Slurm modules. For
> general usage, please consider using:
>
> - [terraform-google-modules/vm/google//modules/compute_instance](https://registry.terraform.io/modules/terraform-google-modules/vm/google/latest/submodules/compute_instance).
> **WARNING:** The source image is not modified. Make sure to use a compatible
> source image.

## Module API

For the terraform module API reference, please see
[README_TF.md](./README_TF.md).

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright (C) SchedMD LLC.
Copyright 2018 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | ~> 1.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.43 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.43 |
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_compute_instance_from_template.slurm_instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance_from_template) | resource |
| [null_resource.replace_trigger](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [google_compute_instance_template.base](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_instance_template) | data source |
| [google_compute_zones.available](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_zones) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_access_config"></a> [access\_config](#input\_access\_config) | Access configurations, i.e. IPs via which the VM instance can be accessed via the Internet. | <pre>list(object({<br/>    nat_ip       = string<br/>    network_tier = string<br/>  }))</pre> | `[]` | no |
| <a name="input_additional_networks"></a> [additional\_networks](#input\_additional\_networks) | Additional network interface details for GCE, if any. | <pre>list(object({<br/>    access_config = optional(list(object({<br/>      nat_ip       = string<br/>      network_tier = string<br/>    })), [])<br/>    alias_ip_range = optional(list(object({<br/>      ip_cidr_range         = string<br/>      subnetwork_range_name = string<br/>    })), [])<br/>    ipv6_access_config = optional(list(object({<br/>      network_tier = string<br/>    })), [])<br/>    network            = optional(string)<br/>    network_ip         = optional(string, "")<br/>    nic_type           = optional(string)<br/>    queue_count        = optional(number)<br/>    stack_type         = optional(string)<br/>    subnetwork         = optional(string)<br/>    subnetwork_project = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_hostname"></a> [hostname](#input\_hostname) | Hostname of instances | `string` | n/a | yes |
| <a name="input_instance_template"></a> [instance\_template](#input\_instance\_template) | Instance template self\_link used to create compute instances | `string` | n/a | yes |
| <a name="input_network"></a> [network](#input\_network) | Network to deploy to. Only one of network or subnetwork should be specified. | `string` | `""` | no |
| <a name="input_num_instances"></a> [num\_instances](#input\_num\_instances) | Number of instances to create. This value is ignored if static\_ips is provided. | `number` | `1` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The GCP project ID | `string` | `null` | no |
| <a name="input_region"></a> [region](#input\_region) | Region where the instances should be created. | `string` | `null` | no |
| <a name="input_replace_trigger"></a> [replace\_trigger](#input\_replace\_trigger) | Trigger value to replace the instances. | `string` | `""` | no |
| <a name="input_static_ips"></a> [static\_ips](#input\_static\_ips) | List of static IPs for VM instances | `list(string)` | `[]` | no |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | Subnet to deploy to. Only one of network or subnetwork should be specified. | `string` | `""` | no |
| <a name="input_subnetwork_project"></a> [subnetwork\_project](#input\_subnetwork\_project) | The project that subnetwork belongs to | `string` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone where the instances should be created. If not specified, instances will be spread across available zones in the region. | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_available_zones"></a> [available\_zones](#output\_available\_zones) | List of available zones in region |
| <a name="output_instances_details"></a> [instances\_details](#output\_instances\_details) | List of all details for compute instances |
| <a name="output_instances_self_links"></a> [instances\_self\_links](#output\_instances\_self\_links) | List of self-links for compute instances |
| <a name="output_names"></a> [names](#output\_names) | List of available zones in region |
| <a name="output_slurm_instances"></a> [slurm\_instances](#output\_slurm\_instances) | List of all resource objects for compute instances |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
