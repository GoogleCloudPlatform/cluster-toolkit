## Description

This module creates a [filestore](https://cloud.google.com/filestore)
instance. Filestore is a high performance network file system that can be
mounted to one or more compute VMs.

For more information on this and other network storage options in the Cloud HPC
Toolkit, see the extended [Network Storage documentation](../../../docs/network_storage.md).

### Filestore tiers

At the time of writing, Filestore supports 5 [tiers of service][tiers] that are
specified in the Toolkit using the following names:

- Basic HDD: "BASIC\_HDD" ([preferred][tierapi]) or "STANDARD" (deprecated)
- Basic SSD: "BASIC\_SSD" ([preferred][tierapi]) or "PREMIUM" (deprecated)
- Zonal With Higher Capacity Band or High Scale SSD: "HIGH\_SCALE\_SSD"
- Enterprise: "ENTERPRISE"
- Zonal With Lower Capacity Band: "ZONAL"

[tierapi]: https://cloud.google.com/filestore/docs/reference/rest/v1beta1/Tier

**Please review the minimum storage requirements for each tier**. The Terraform
module can only enforce the minimum value of the `size_gb` parameter for the
lowest tier of service. If you supply a value that is too low, Filestore
creation will fail when you run `terraform apply`.

[tiers]: https://cloud.google.com/filestore/docs/service-tiers

### Filestore mount options
After Filestore instance is created, you can mount this to the compute node
using different mount options. Toolkit uses [default mount options](https://linux.die.net/man/8/mount)
for all tier services. Filestore has recommended mount options for different
service tiers which may overall improve performance. These can be found here:
[recommended mount options.](https://cloud.google.com/filestore/docs/mounting-fileshares)
While creating filestore module, you can overwrite these mount options as
mentioned below.

```yaml
- id: homefs
  source: modules/file-system/filestore
  use: [network1]
  settings:
    local_mount: /homefs
    mount_options: defaults,hard,timeo=600,retrans=3,_netdev
```

### Filestore quota

Your project must have unused quota for Cloud Filestore in the region you will
provision the storage. This can be found by browsing to the [Quota tab within IAM
& Admin](https://console.cloud.google.com/iam-admin/quotas) in the Cloud Console.
Please note that there are separate quota limits for HDD and SSD storage.

All projects begin with 0 available quota for High Scale SSD tier. To use this
tier, [make a request and wait for it to be approved][hs-ssd-quota].

[hs-ssd-quota]: https://cloud.google.com/filestore/docs/high-scale

### Example - Basic HDD

The Filestore instance defined below will have the following attributes:

- (default) `BASIC_HDD` tier
- (default) 1TiB capacity
- `homefs` module ID
- mount point at `/home`
- connected to the network defined in the `network1` module

```yaml
- id: homefs
  source: modules/file-system/filestore
  use: [network1]
  settings:
    local_mount: /home
```

### Example - High Scale SSD

The Filestore instance defined below will have the following attributes:

- `HIGH_SCALE_SSD` tier
- 10TiB capacity
- `highscale` module ID
- mount point at `/projects`
- connected to the VPC network defined in the `network1` module

```yaml
- id: highscale
  source: modules/file-system/filestore
  use: [network1]
  settings:
    filestore_tier: HIGH_SCALE_SSD
    size_gb: 10240
    local_mount: /projects
```

## Mounting

To mount the Filestore instance you must first ensure that the NFS client has
been installed and then call the proper `mount` command.

Both of these steps are automatically handled with the use of the `use` command
in a selection of HPC Toolkit modules. See the [compatibility matrix][matrix] in
the network storage doc for a complete list of supported modules.
See the [hpc-slurm](../../../examples/hpc-slurm.yaml) for
an example of using this module with Slurm.

If mounting is not automatically handled as described above, the `filestore`
module outputs runners that can be used with the startup-script module to
install the client and mount the file system. See the following example:

```yaml
  - id: filestore
    source: modules/file-system/filestore
    use: [network1]
    settings: {local_mount: /scratch}

  - id: mount-at-startup
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(filestore.install_nfs_client_runner)
      - $(filestore.mount_runner)

```

[matrix]: ../../../docs/network_storage.md#compatibility-matrix

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
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 4.19 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 4.19 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_filestore_instance.filestore_instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/filestore_instance) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_connect_mode"></a> [connect\_mode](#input\_connect\_mode) | Used to select mode - supported values DIRECT\_PEERING and PRIVATE\_SERVICE\_ACCESS. | `string` | `"DIRECT_PEERING"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used as name of the filestore instance if no name is specified. | `string` | n/a | yes |
| <a name="input_filestore_share_name"></a> [filestore\_share\_name](#input\_filestore\_share\_name) | Name of the file system share on the instance. | `string` | `"nfsshare"` | no |
| <a name="input_filestore_tier"></a> [filestore\_tier](#input\_filestore\_tier) | The service tier of the instance. | `string` | `"BASIC_HDD"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the filestore instance. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Mountpoint for this filestore instance. Note: If set to the same as the `filestore_share_name`, it will trigger a known Slurm bug ([troubleshooting](../../../docs/slurm-troubleshooting.md)). | `string` | `"/shared"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | NFS mount options to mount file system. | `string` | `"defaults,_netdev"` | no |
| <a name="input_name"></a> [name](#input\_name) | The resource name of the instance. | `string` | `null` | no |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the instance is connected given in the format:<br>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which Filestore instance will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Location for Filestore instances at Enterprise tier. | `string` | n/a | yes |
| <a name="input_reserved_ip_range"></a> [reserved\_ip\_range](#input\_reserved\_ip\_range) | Reserved IP range for Filestore instance (set to null to enable automatic selection) | `string` | `null` | no |
| <a name="input_size_gb"></a> [size\_gb](#input\_size\_gb) | Storage size of the filestore instance in GB. | `number` | `1024` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Location for Filestore instances below Enterprise tier. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_capacity_gb"></a> [capacity\_gb](#output\_capacity\_gb) | File share capacity in GiB. |
| <a name="output_filestore_id"></a> [filestore\_id](#output\_filestore\_id) | An identifier for the resource with format `projects/{{project}}/locations/{{location}}/instances/{{name}}` |
| <a name="output_install_nfs_client"></a> [install\_nfs\_client](#output\_install\_nfs\_client) | Script for installing NFS client |
| <a name="output_install_nfs_client_runner"></a> [install\_nfs\_client\_runner](#output\_install\_nfs\_client\_runner) | Runner to install NFS client using the startup-script module |
| <a name="output_mount_runner"></a> [mount\_runner](#output\_mount\_runner) | Runner to mount the file-system using an ansible playbook. The startup-script<br>module will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-fs-id.mount\_runner)<br>... |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a filestore instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
