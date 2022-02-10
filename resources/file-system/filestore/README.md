## Description

This resource creates a [filestore](https://cloud.google.com/filestore)
instance. Filestore is a high performance network file system that can be
mounted to one or more compute VMs.

### Example - PREMIUM tier

```yaml
- source: ./resources/file-system/filestore
  kind: terraform
  id: homefs
  settings:
    local_mount: /home
    network_name: $(network1.network_name)
    size_gb: 2660
```

This creates a filestore instance with the resource ID of `homefs` that will be
mounted at `/home` and is connected to the network defined in the `network1`
resource. `size_gb` is set to the minimum for the default "PREMIUM" tier,
2.5TiB.

### Example - High Scale

```yaml
- source: ./resources/file-system/filestore
  kind: terraform
  id: highscale
  settings:
    filestore_tier: HIGH_SCALE_SSD
    size_gb: 10240
    local_mount: /projects
    network_name: $(network1.network_name)
```

This creates a high scale filestore instance that will be mounted at
`/projects` and has the minimum capacity for a high scale SDD instance of 10TiB.

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
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | ~> 3.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | ~> 3.0 |
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
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used as name of the filestore instace if no name is specified. | `string` | n/a | yes |
| <a name="input_filestore_share_name"></a> [filestore\_share\_name](#input\_filestore\_share\_name) | Name of the file system share on the instance. | `string` | `"nfsshare"` | no |
| <a name="input_filestore_tier"></a> [filestore\_tier](#input\_filestore\_tier) | The service tier of the instance. | `string` | `"PREMIUM"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the filestore instance. List key, value pairs. | `any` | n/a | yes |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Mountpoint for this filestore instance. | `string` | `"/shared"` | no |
| <a name="input_name"></a> [name](#input\_name) | The resource name of the instance. | `string` | `null` | no |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | The name of the GCE VPC network to which the instance is connected. | `string` | n/a | yes |
| <a name="input_size_gb"></a> [size\_gb](#input\_size\_gb) | Storage size of the filestore instance in GB. | `number` | `2660` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The name of the Filestore zone of the instance. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_install_nfs_client"></a> [install\_nfs\_client](#output\_install\_nfs\_client) | Script for installing NFS client |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a filestore instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
