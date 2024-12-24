## Description

This module defines a file-system that already exists (i.e. it does not create
a new file system) in a way that can be shared with other modules. This allows
a compute VM to mount a filesystem that is not part of the current deployment
group.

The pre-existing network storage can be referenced in the same way as any Cluster
Toolkit supported file-system such as [filestore](../filestore/README.md).

For more information on network storage options in the Cluster Toolkit, see
the extended [Network Storage documentation](../../../../docs/network_storage.md).

### Example

```yaml
- id: homefs
  source: modules/file-system/pre-existing-network-storage
  settings:
    server_ip: ## Set server IP here ##
    remote_mount: nfsshare
    local_mount: /home
    fs_type: nfs
```

This creates a pre-existing-network-storage module in terraform at the
provided IP in `server_ip` of type nfs that will be mounted at `/home`. Note
that the `server_ip` must be known before deployment.

The following is an example of using `pre-existing-network-storage` with a GCS
bucket:

```yaml
- id: data-bucket
  source: modules/file-system/pre-existing-network-storage
  settings:
    remote_mount: my-bucket-name
    local_mount: /data
    fs_type: gcsfuse
    mount_options: defaults,_netdev,implicit_dirs
```

The `implicit_dirs` mount option allows object paths to be treated as if they
were directories. This is important when working with files that were created by
another source, but there may have performance impacts. The `_netdev` mount option
denotes that the storage device requires network access.

The following is an example of using `pre-existing-network-storage` with the `lustre`
filesystem:

```yaml
- id: lustrefs
  source: modules/file-system/pre-existing-network-storage
  settings:
    fs_type: lustre
    server_ip: 192.168.227.11@tcp
    local_mount: /scratch
    remote_mount: /exacloud
```

Note the use of the MGS NID (Network ID) in the `server_ip` field - in particular, note the `@tcp` suffix.

The following is an example of using `pre-existing-network-storage` with the `daos`
filesystem. In order to use existing `parallelstore` instance, `fs_type` needs to be
explicitly mentioned in blueprint. The `remote_mount` option refers to `access_points`
for `parallelstore` instance.

```yaml
- id: parallelstorefs
  source: modules/file-system/pre-existing-network-storage
  settings:
    fs_type: daos
    remote_mount: "[10.246.99.2,10.246.99.3,10.246.99.4]"
    mount_options: disable-wb-cache,thread-count=16,eq-count=8
```

Parallelstore supports additional options for its mountpoints under `parallelstore_options` setting.
Use `daos_agent_config` to provide additional configuration for `daos_agent`, for example:

```yaml
- id: parallelstorefs
  source: modules/file-system/pre-existing-network-storage
  settings:
    fs_type: daos
    remote_mount: "[10.246.99.2,10.246.99.3,10.246.99.4]"
    mount_options: disable-wb-cache,thread-count=16,eq-count=8
    parallelstore_options:
      daos_agent_config: |
        credential_config:
          cache_expiration: 1m
```

Use `dfuse_environment` to provide additional environment variables for `dfuse` process, for example:

```yaml
- id: parallelstorefs
  source: modules/file-system/pre-existing-network-storage
  settings:
    fs_type: daos
    remote_mount: "[10.246.99.2,10.246.99.3,10.246.99.4]"
    mount_options: disable-wb-cache,thread-count=16,eq-count=8
    parallelstore_options:
      dfuse_environment:
        D_LOG_FILE: /tmp/client.log
        D_APPEND_PID_TO_LOG: 1
        D_LOG_MASK: debug
```

### Mounting

For the `fs_type` listed below, this module will provide `client_install_runner`
and `mount_runner` outputs. These can be used to create a startup script to
mount the network storage system.

Supported `fs_type`:

- nfs
- lustre
- gcsfuse
- daos

[scripts/mount.sh](./scripts/mount.sh) is used as the contents of
`mount_runner`. This script will update `/etc/fstab` and mount the network
storage. This script will fail if the specified `local_mount` is already being
used by another entry in `/etc/fstab`.

Both of these steps are automatically handled with the use of the `use` command
in a selection of Cluster Toolkit modules. See the [compatibility matrix][matrix] in
the network storage doc for a complete list of supported modules.

[matrix]: ../../../docs/network_storage.md#compatibility-matrix

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
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Options describing various aspects of the file system. Consider adding setting to 'defaults,\_netdev,implicit\_dirs' when using gcsfuse. | `string` | `"defaults,_netdev"` | no |
| <a name="input_parallelstore_options"></a> [parallelstore\_options](#input\_parallelstore\_options) | Parallelstore specific options | <pre>object({<br/>    daos_agent_config = optional(string, "")<br/>    dfuse_environment = optional(map(string), {})<br/>  })</pre> | `{}` | no |
| <a name="input_remote_mount"></a> [remote\_mount](#input\_remote\_mount) | Remote FS name or export. This is the exported directory for nfs, fs name for lustre, and bucket name (without gs://) for gcsfuse. | `string` | n/a | yes |
| <a name="input_server_ip"></a> [server\_ip](#input\_server\_ip) | The device name as supplied to fs-tab, excluding remote fs-name(for nfs, that is the server IP, for lustre <MGS NID>[:<MGS NID>]). This can be omitted for gcsfuse. | `string` | `""` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_client_install_runner"></a> [client\_install\_runner](#output\_client\_install\_runner) | Runner that performs client installation needed to use file system. |
| <a name="output_mount_runner"></a> [mount\_runner](#output\_mount\_runner) | Runner that mounts the file system. |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a remote network storage to be mounted by fs-tab. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
