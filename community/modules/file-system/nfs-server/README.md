## Description

This module creates a Network File Sharing (NFS) file system based on a VM
instance and [compute disk][disk]. This file system can share directories and
files with other clients over a network. `nfs-server` can be used by
[vm-instance](../../../../modules/compute/vm-instance/README.md) and SchedMD
community modules that create compute VMs.

For more information on this and other network storage options in the Cloud HPC
Toolkit, see the extended [Network Storage documentation](../../../../docs/network_storage.md).

> **_WARNING:_** This module has only been tested against the HPC centos7 OS
> disk image (the default). Using other images may work, but have not been
> verified.

[disk]: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk

### Example

```yaml
- id: homefs
  source: community/modules/file-system/nfs-server
  use: [network1]
  settings:
    auto_delete_disk: true
```

This creates a NFS on a virtual machine which allow other VMs to mount the
volume as an external file system.

> **_NOTE:_** `auto_delete_disk` is set to true in this example, which means
> that running `terraform destroy` also deletes the disk. To retain the disk
> after `terraform destroy` either set this to false or don't include the
> settings so it defaults to false. Note that with `auto_delete_disk: false`,
> you will need to manually delete the disk after destroying a deployment group
> with `nfs-server`.

## Mounting

To mount the NFS Server you must first ensure that the NFS client has been
installed the and then call the proper `mount` command.

Both of these steps are automatically handled with the use of the `use` command
in a selection of HPC Toolkit modules. See the [compatibility matrix][matrix] in
the network storage doc for a complete list of supported modules.
See the [hpc-centos-ss.yaml] test config for an example of using this module
with a `vm-instance` module.

If mounting is not automatically handled as described above, the `nfs-server`
module outputs runners that can be used with the startup-script module to
install the client and mount the file system. See the following example:

```yaml
  - id: nfs
    source: community/modules/file-system/nfs-server
    use: [network1]
    settings: {local_mounts: [/mnt1]}

  - id: mount-at-startup
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(nfs.install_nfs_client_runner)
      - $(nfs.mount_runner)

```

[hpc-centos-ss.yaml]: ../../../../tools/validate_configs/test_configs/hpc-centos-ss.yaml
[matrix]: ../../../../docs/network_storage.md#compatibility-matrix

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
| <a name="requirement_null"></a> [null](#requirement\_null) | >= 3.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_null"></a> [null](#provider\_null) | >= 3.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_compute_disk.attached_disk](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk) | resource |
| [google_compute_disk.boot_disk](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk) | resource |
| [google_compute_instance.compute_instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance) | resource |
| [null_resource.image](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [google_compute_default_service_account.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_default_service_account) | data source |
| [google_compute_image.compute_image](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_image) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_auto_delete_disk"></a> [auto\_delete\_disk](#input\_auto\_delete\_disk) | Whether or not the nfs disk should be auto-deleted | `bool` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used as name of the NFS instace if no name is specified. | `string` | n/a | yes |
| <a name="input_disk_size"></a> [disk\_size](#input\_disk\_size) | Storage size gb | `number` | `"100"` | no |
| <a name="input_image"></a> [image](#input\_image) | DEPRECATED: The VM image used by the nfs server | `string` | `null` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | The VM image used by the nfs server.<br><br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted. | `map(string)` | <pre>{<br>  "family": "hpc-centos-7",<br>  "project": "cloud-hpc-image-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the NFS instance. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_local_mounts"></a> [local\_mounts](#input\_local\_mounts) | Mountpoint for this NFS compute instance | `list(string)` | <pre>[<br>  "/data"<br>]</pre> | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Type of the VM instance to use | `string` | `"n2d-standard-2"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map | `map(string)` | `{}` | no |
| <a name="input_name"></a> [name](#input\_name) | The resource name of the instance. | `string` | `null` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network to attach the nfs VM. | `string` | `"default"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_scopes"></a> [scopes](#input\_scopes) | Scopes to apply to the controller | `list(string)` | <pre>[<br>  "https://www.googleapis.com/auth/cloud-platform"<br>]</pre> | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service Account for the NFS Server | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork to attach the nfs VM. | `string` | `null` | no |
| <a name="input_type"></a> [type](#input\_type) | The service tier of the instance. | `string` | `"pd-ssd"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The zone name where the nfs instance located in. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_install_nfs_client"></a> [install\_nfs\_client](#output\_install\_nfs\_client) | Script for installing NFS client |
| <a name="output_install_nfs_client_runner"></a> [install\_nfs\_client\_runner](#output\_install\_nfs\_client\_runner) | Runner to install NFS client using the startup-script module |
| <a name="output_mount_runner"></a> [mount\_runner](#output\_mount\_runner) | Runner to mount the file-system using an ansible playbook. The startup-script<br>module will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-fs-id.mount\_runner)<br>... |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | export of all desired folder directories |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
