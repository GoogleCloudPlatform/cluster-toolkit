## Description

This resource creates a Network File Sharing (NFS) disk to share directories and files  with other clients over a network via the [Terraform Google Documentation](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk )   to be mounted upon a google compute engine instance created through the [Terraform Google Documentation](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance ).   

### Example

```
- source: resources/file-system/nfs-server
  kind: terraform
  id: homefs
  settings:
    network_name: $(network1.network_name)
    labels:
      ghpc_role: storage-home
```

This creates a NFS on a virtual machine which allow other VMs to mount the volume as an external file system.

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

No modules.

## Resources

| Name | Type |
|------|------|
| [google_compute_disk.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk) | resource |
| [google_compute_instance.compute_instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_auto_delete_disk"></a> [auto\_delete\_disk](#input\_auto\_delete\_disk) | Whether or not the nfs disk should be auto-deleted | `bool` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used as name of the filestore instace if no name is specified. | `string` | n/a | yes |
| <a name="input_disk_size"></a> [disk\_size](#input\_disk\_size) | Storage size gb | `number` | `"100"` | no |
| <a name="input_image_family"></a> [image\_family](#input\_image\_family) | the VM image used by the nfs server | `string` | `"centos-7"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the NFS instance. List key, value pairs. | `any` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Type of the VM instance to use | `string` | `"n2d-standard-2"` | no |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | Network to deploy to. Only one of network or subnetwork should be specified. | `string` | `"default"` | no |
| <a name="input_network_project"></a> [network\_project](#input\_network\_project) | the project where the shared network locates in | `string` | n/a | yes |
| <a name="input_type"></a> [type](#input\_type) | The service tier of the instance. | `string` | `"pd-ssd"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The name of the Filestore zone of the instance. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a nfs instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->