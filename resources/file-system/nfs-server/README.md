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
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 3.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [ "google_compute_instance" "compute_instance" ](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance) | resource |

## Inputs

| Name                                                         | Description                                                  | Type     | Default            |
| :----------------------------------------------------------- | ------------------------------------------------------------ | -------- | ------------------ |
| <a name="input_project_id"></a> [project\_id](#input\\_project\) | Project id under which the nfs will be created               | `string` | n/a                |
| <a name="input_zone"></a> [zone](#input\_display\_name)      | The name of the zone                                         | `string` | us-central1-f      |
| <a name="input_disk_size"></a> [disk_size](#input\_disk_size\) | Mention the size of the disk                                 | `string` | 100                |
| <a name="input_type"></a> [type](#input\_type)               | Type of disk that can be used                                | `string` | pd-ssd             |
| <a name="input_image_family"></a> [image_family](#input\_image_family) | Type of OS image that can be used                            | `string` | centos-7           |
| <a name="input_auto_delete_disk"></a> [auto_delete_disk](#input\_auto_delete_disk) | Whether or not the boot disk should be auto-deleted          | `bool`   | false              |
| <a name="input_region"></a> [region](#input\_region)         | Region where the instances should be created                 | `string` | us-central1        |
| <a name="input_network_name"></a> [network_name](#input\_filestore_disk_threshold) | Network to deploy to. Only one of network or subnetwork should be specified | `string` | default            |
| <a name="input_name_prefix"></a> [name_prefix](#input\_name\_prefix) | The name prefix for the resources                            | `string` | hpc-nfs            |
| <a name="input_machine_type"></a> [machine_type](#input\_machine_type) | Type of the VM instance to use                               | `string` | n2d-standard-2     |
| <a name="input_network_tier"></a> [network_tier](#input\_network_tier) | The threshold value at which alert will be sent if the number of storage requests exceeds | `string` | PREMIUM            |
| <a name="input_export_paths"></a> [export_paths](#input\export_paths) | Paths to exports                                             | `string` | "/home/", "/tools" |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_tools-volume-ip-addresses"></a> [tools-volume-ip-addresses](#output\_tools-volume-ip-addresses) | Describes the mounted volume's IP address |
| <a name="output_home-volume-ip-addresses"></a> [home-volume-ip-addresses](#output\_home-volume-ip-addresses) | Describes the mounted volume's IP address |
|<a name="output_network_storage"></a> [network_storage](#output\_network_storage)|Describes a nfs instance|
|<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->||