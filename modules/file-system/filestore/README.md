## Description

This module creates a [filestore](https://cloud.google.com/filestore)
instance. Filestore is a high performance network file system that can be
mounted to one or more compute VMs.

### Filestore tiers

At the time of writing, Filestore supports 4 [tiers of service][tiers] that are
specified in the Toolkit using the following names:

- Basic HDD: "BASIC\_HDD" ([preferred][tierapi]) or "STANDARD" (deprecated)
- Basic SSD: "BASIC\_SSD" ([preferred][tierapi]) or "PREMIUM" (deprecated)
- High Scale SSD: "HIGH\_SCALE\_SSD"
- Enterprise: "ENTERPRISE"

[tierapi]: https://cloud.google.com/filestore/docs/reference/rest/v1beta1/Tier

**Please review the minimum storage requirements for each tier**. The Terraform
module can only enforce the minimum value of the `size_gb` parameter for the
lowest tier of service. If you supply a value that is too low, Filestore
creation will fail when you run `terraform apply`.

[tiers]: https://cloud.google.com/filestore/docs/service-tiers

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
- source: ./modules/file-system/filestore
  kind: terraform
  id: homefs
  settings:
    local_mount: /home
    network_name: $(network1.network_name)
```

### Example - High Scale SSD

The Filestore instance defined below will have the following attributes:

- `HIGH_SCALE_SSD` tier
- 10TiB capacity
- `highscale` module ID
- mount point at `/projects`
- connected to the VPC network defined in the `network1` module

```yaml
- source: ./modules/file-system/filestore
  kind: terraform
  id: highscale
  settings:
    filestore_tier: HIGH_SCALE_SSD
    size_gb: 10240
    local_mount: /projects
    network_name: $(network1.network_name)
```

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
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 4.4 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 4.4 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_filestore_instance.filestore_instance](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_filestore_instance) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_connect_mode"></a> [connect\_mode](#input\_connect\_mode) | Used to select mode - supported values DIRECT\_PEERING and PRIVATE\_SERVICE\_ACCESS. | `string` | `"DIRECT_PEERING"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used as name of the filestore instace if no name is specified. | `string` | n/a | yes |
| <a name="input_filestore_share_name"></a> [filestore\_share\_name](#input\_filestore\_share\_name) | Name of the file system share on the instance. | `string` | `"nfsshare"` | no |
| <a name="input_filestore_tier"></a> [filestore\_tier](#input\_filestore\_tier) | The service tier of the instance. | `string` | `"BASIC_HDD"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the filestore instance. List key, value pairs. | `any` | n/a | yes |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Mountpoint for this filestore instance. | `string` | `"/shared"` | no |
| <a name="input_name"></a> [name](#input\_name) | The resource name of the instance. | `string` | `null` | no |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | The name of the GCE VPC network to which the instance is connected. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which Filestore instance will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Location for Filestore instances at Enterprise tier. | `string` | n/a | yes |
| <a name="input_size_gb"></a> [size\_gb](#input\_size\_gb) | Storage size of the filestore instance in GB. | `number` | `1024` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Location for Filestore instances below Enterprise tier. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_install_nfs_client"></a> [install\_nfs\_client](#output\_install\_nfs\_client) | Script for installing NFS client |
| <a name="output_install_nfs_client_runner"></a> [install\_nfs\_client\_runner](#output\_install\_nfs\_client\_runner) | Runner to install NFS client using the startup-script module |
| <a name="output_mount_runner"></a> [mount\_runner](#output\_mount\_runner) | Runner to mount the file-system using the startup-script module.<br>This runner requires ansible to be installed. This can be achieved using the<br>install\_ansible.sh script as a prior runner in the startup-script module:<br>runners:<br>- type: shell<br>  source: modules/startup-script/examples/install\_ansible.sh<br>  destination: install\_ansible.sh<br>- $(your-fs-id.mount\_runner)<br>... |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a filestore instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
