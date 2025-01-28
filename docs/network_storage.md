# Network Storage in the Cluster Toolkit (formerly HPC Toolkit)

The Cluster Toolkit provides powerful tools for working with network
storage.

The Toolkit contains modules that will **provision**:

- [Filestore (GCP managed NFS)][filestore]
- [DDN EXAScaler lustre][ddn-exascaler]
- [Parallelstore][parallelstore]
- [NFS server (non-GCP managed)][nfs-server]

The Toolkit also provides a **[pre-existing-network-storage]** module to work
with a network storage device that is already set up. The
`pre-existing-network-storage` module supports the following file systems types:

- nfs
- lustre
- gcsfuse

## Connecting to Network Storage

In addition to provisioning a network storage device, most file system modules
contain scripts that will install any required software needed to utilize the
device and mount the file system automatically on
[supported and tested VM images](./vm-images.md).

### Mounting Via Use

The simplest way to mount a network storage device is by using the `use` field,
as shown below:

```yaml
...
  - id: network1
    source: modules/network/vpc

  - id: homefs
    source: modules/file-system/filestore
    use: [network1]
    settings: {local_mount: /home}

  - id: workstation
    source: modules/compute/vm-instance
    use: [network1, homefs]  # Note this line
```

The example above is creating a filestore and automatically mounting it to a VM.
Take note of the line with the comment saying `# Note this line`. By adding the
`homefs` id to the `use` field of `workstation` several things automatically
happen:

- The `homefs` filestore outputs scripts for nfs client installation and
  mounting.
- The `workstation` VM reads these scripts and generates a startup script for
  the VM.
- The VM startup script will automatically install the nfs client on its first
  boot (if not already installed).
- The VM will add `homefs` to fstab and mount the file system.

This same pattern works across most modules in the toolkit. The
[compatibility matrix](#compatibility-matrix) below shows modules that can use
this method.

### Mounting Via Startup

Not all mounting scenarios are supported via the `use` filed. When `use` is not
supported, automated client installation and mounting can be accomplished by
using the `startup-script` module. Use the
[compatibility matrix](#compatibility-matrix) to determine when this method is
needed.

The following is an example setting up a filestore using startup script:

```yaml
...
  - id: network1
    source: modules/network/vpc

  - id: homefs
    source: modules/file-system/filestore
    use: [network1]
    settings: {local_mount: /home}

  - id: filestore-setup
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(homefs.install_nfs_client_runner)
      - $(homefs.mount_runner)
```

> **_NOTE:_** The exact names of the runners may be different from module to
> module.

## Compatibility Matrix

The following matrix shows the best method by which each type of network storage
device should be mounted to each mount capable module.

&nbsp; | Slurm V6 | Slurm V5 | Batch | vm-instance | Packer (client install) | HTCondor\* | PBS Pro\*
-- | -- | -- | -- | -- | -- | -- | --
filestore | via USE | via USE | via USE | via USE | via STARTUP | via USE | via USE
nfs-server | via USE | via USE | via USE | via USE | via STARTUP | via USE | via USE
cloud-storage-bucket (GCS)| via USE | via USE | via USE | via USE | via STARTUP | via USE | via USE
DDN EXAScaler lustre | via USE | via USE | via USE | via USE | Needs Testing | via USE | via USE
Parallelstore | via USE | Needs Testing | Needs Testing | via USE | Needs Testing | Needs Testing | Needs Testing
  |  |   |   |   |   |   |  
filestore (pre-existing) | via USE | via USE | via USE | via USE | via STARTUP | via USE | via USE
nfs-server (pre-existing) | via USE | via USE | via USE | via USE | via STARTUP | via USE | via USE
DDN EXAScaler lustre (pre-existing) | via USE | via USE | via USE | via USE | Needs Testing | via USE | via USE
Parallelstore (pre-existing) | via USE | Needs Testing | Needs Testing | via USE | Needs Testing | Needs Testing | Needs Testing
GCS FUSE (pre-existing) | via USE | via USE | via USE | via USE | via STARTUP | via USE | Needs Testing

- **via USE:** Client installation and mounting occur automatically when
  connected with the use field. See
  [mounting via use section](#mounting-via-use).
- **via STARTUP:** Startup scripts are provided that can be used with the
  `startup-script` module to install clients and mount. See
  [mounting via startup section](#mounting-via-startup).
- **Needs Testing:** May currently work but has not yet been fully tested.
- **Not Supported:** This feature is not supported right now.

\* only supported on CentOS 7\

[filestore]: ../modules/file-system/filestore/README.md
[pre-existing-network-storage]: ../modules/file-system/pre-existing-network-storage/README.md
[ddn-exascaler]: ../community/modules/file-system/DDN-EXAScaler/README.md
[parallelstore]: ../modules/file-system/parallelstore/README.md
[nfs-server]: ../community/modules/file-system/nfs-server/README.md
