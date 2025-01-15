# Module: Slurm Nodeset (TPU)

[FAQ](../../../../docs/faq.md) |
[Troubleshooting](../../../../docs/troubleshooting.md) |
[Glossary](../../../../docs/glossary.md)

<!-- mdformat-toc start --slug=github --no-anchors --maxlevel=6 --minlevel=1 -->

- [Module: Slurm Nodeset (TPU)](#module-slurm-nodeset-tpu)
  - [Overview](#overview)
  - [Module API](#module-api)

<!-- mdformat-toc end -->

## Overview

This is a submodule of [slurm_cluster](../../../slurm_cluster/README.md). It
creates a Slurm TPU nodeset for [slurm_partition](../slurm_partition/README.md).

## Module API

For the terraform module API reference, please see
[README_TF.md](./README_TF.md).

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright (C) SchedMD LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | ~> 1.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.53 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.53 |
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [null_resource.nodeset_tpu](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [google_compute_subnetwork.nodeset_subnetwork](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_accelerator_config"></a> [accelerator\_config](#input\_accelerator\_config) | Nodeset accelerator config, see https://cloud.google.com/tpu/docs/supported-tpu-configurations for details. | <pre>object({<br/>    topology = string<br/>    version  = string<br/>  })</pre> | <pre>{<br/>  "topology": "",<br/>  "version": ""<br/>}</pre> | no |
| <a name="input_data_disks"></a> [data\_disks](#input\_data\_disks) | The data disks to include in the TPU node | `list(string)` | `[]` | no |
| <a name="input_docker_image"></a> [docker\_image](#input\_docker\_image) | The gcp container registry id docker image to use in the TPU vms, it defaults to gcr.io/schedmd-slurm-public/tpu:slurm-gcp-6-8-tf-<var.tf\_version> | `string` | `""` | no |
| <a name="input_enable_public_ip"></a> [enable\_public\_ip](#input\_enable\_public\_ip) | Enables IP address to access the Internet. | `bool` | `false` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on nodes. | <pre>list(object({<br/>    server_ip     = string,<br/>    remote_mount  = string,<br/>    local_mount   = string,<br/>    fs_type       = string,<br/>    mount_options = string,<br/>  }))</pre> | `[]` | no |
| <a name="input_node_count_dynamic_max"></a> [node\_count\_dynamic\_max](#input\_node\_count\_dynamic\_max) | Maximum number of nodes allowed in this partition to be created dynamically. | `number` | `0` | no |
| <a name="input_node_count_static"></a> [node\_count\_static](#input\_node\_count\_static) | Number of nodes to be statically created. | `number` | `0` | no |
| <a name="input_node_type"></a> [node\_type](#input\_node\_type) | Specify a node type to base the vm configuration upon it. Not needed if you use accelerator\_config | `string` | `null` | no |
| <a name="input_nodeset_name"></a> [nodeset\_name](#input\_nodeset\_name) | Name of Slurm nodeset. | `string` | n/a | yes |
| <a name="input_preemptible"></a> [preemptible](#input\_preemptible) | Specify whether TPU-vms in this nodeset are preemtible, see https://cloud.google.com/tpu/docs/preemptible for details. | `bool` | `false` | no |
| <a name="input_preserve_tpu"></a> [preserve\_tpu](#input\_preserve\_tpu) | Specify whether TPU-vms will get preserve on suspend, if set to true, on suspend vm is stopped, on false it gets deleted | `bool` | `true` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_reserved"></a> [reserved](#input\_reserved) | Specify whether TPU-vms in this nodeset are created under a reservation. | `bool` | `false` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the TPU-vm.<br/>If none is given, the default service account and scopes will be used. | <pre>object({<br/>    email  = string<br/>    scopes = set(string)<br/>  })</pre> | `null` | no |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | The name of the subnetwork to attach the TPU-vm of this nodeset to. | `string` | n/a | yes |
| <a name="input_tf_version"></a> [tf\_version](#input\_tf\_version) | Nodeset Tensorflow version, see https://cloud.google.com/tpu/docs/supported-tpu-configurations#tpu_vm for details. | `string` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | Nodes will only be created in this zone. Check https://cloud.google.com/tpu/docs/regions-zones to get zones with TPU-vm in it. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset"></a> [nodeset](#output\_nodeset) | Nodeset details. |
| <a name="output_nodeset_name"></a> [nodeset\_name](#output\_nodeset\_name) | Nodeset name. |
| <a name="output_service_account"></a> [service\_account](#output\_service\_account) | Service account object, includes email and scopes. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
