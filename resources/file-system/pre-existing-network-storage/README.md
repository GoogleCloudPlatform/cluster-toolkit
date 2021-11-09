## Description
This resource defines a file-system that already exists (i.e. it does not create
a new file system) in a way that can be shared with other resources. This allows
a compute VM to mount a filesystem that is not a part of the current deployment.

The pre-existing network storage can be referenced in the same way as any HPC
Toolkit supported file-system such as [filestore](../filestore/README.md).

### Example
```
- source: ./resources/file-system/pre-existing-network-storage
  kind: terraform
  id: homefs
  settings:
    server_ip: ## Set server IP here ##
    remote_mount: nfsshare
    local_mount: /home
    fs_type: nfs
```
This creates a pre-existing-network-storage resource in terraform at the
provided IP in `server_ip` of type nfs that will be mounted at /home. Note that
the `server_ip` must be known before deployment.

## License
<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_fs_type"></a> [fs\_type](#input\_fs\_type) | Type of file system to be mounted (e.g., nfs, lustre) | `string` | `"nfs"` | no |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | The mount point where the contents of the device may be accessed after mounting. | `string` | `"/mnt"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Options describing various aspects of the file system. | `string` | `""` | no |
| <a name="input_remote_mount"></a> [remote\_mount](#input\_remote\_mount) | Remote FS name or export (exported directory for nfs, fs name for lustre) | `string` | n/a | yes |
| <a name="input_server_ip"></a> [server\_ip](#input\_server\_ip) | The device name as supplied to fs-tab, excluding remote fs-name(for nfs, that is the server IP, for lustre <MGS NID>[:<MGS NID>]). | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a remote network storage to be mounted by fs-tab. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
