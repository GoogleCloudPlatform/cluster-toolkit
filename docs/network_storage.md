# Network Storage in the Cluster Toolkit (formerly HPC Toolkit)

The Cluster Toolkit provides powerful tools for working with network
storage.

The Toolkit contains modules that will **provision**:

- [Google Cloud NetApp Volumes (GCP managed enterprise NFS)][netapp-volumes]
- [Filestore (GCP managed NFS)][filestore]
- [Managed Lustre][managed-lustre]
- [NFS server (non-GCP managed)][nfs-server]

The Toolkit also provides a **[pre-existing-network-storage]** module to work
with a network storage device that is already set up. The
`pre-existing-network-storage` module supports the following file systems types:

- nfs
- daos
- managed-lustre
- gcsfuse

## NetApp Volumes (Default mode)

The [netapp-storage-pool][netapp-storage-pool] and [netapp-volume][netapp-volumes] modules provision NetApp Volumes in Default mode with NFSv3 and NFSv4.1 only. If you need ONTAP-mode pools or volumes, provision them outside Cluster Toolkit and use [pre-existing-network-storage] to mount the NFS export in your blueprint.

### Service levels

1. **STANDARD, PREMIUM, EXTREME** — Regional pools. Set `region` only.
2. **Flex Unified** — Zonal or regional pools. Set `region` and `zone`; add `replica_zone` for regional pools. Optionally set `total_throughput_mibps` and `total_iops` for custom performance.

### Capacity minimums

Storage pools:

1. STANDARD, PREMIUM, EXTREME — 2048 GiB
2. Flex Unified — 1024 GiB
3. Flex Unified large capacity (`SCALE_TYPE_SCALEOUT`) — 6144 GiB

Volumes:

1. STANDARD, PREMIUM, EXTREME (regular) — 100 GiB
2. STANDARD, PREMIUM, EXTREME (large capacity) — 15 TiB (15360 GiB); set `large_capacity: true`
3. Flex Unified — 1 GiB
4. Flex Unified large capacity — 4800 GiB; set `large_capacity_config.constituent_count`

### Not supported by these modules

1. Flex File
2. ONTAP mode (use [pre-existing-network-storage])
3. SMB and iSCSI

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

&nbsp; | Slurm V6 | Batch | vm-instance | Packer (client install) | HTCondor\*
-- | -- | -- | -- | -- | --
filestore | via USE | via USE | via USE | via STARTUP | via USE
nfs-server | via USE | via USE | via USE | via STARTUP | via USE
cloud-storage-bucket (GCS)| via USE | via USE | via USE | via STARTUP | via USE
Managed Lustre | via USE | Needs Testing | via USE | Needs Testing | Needs Testing
netapp-volume | via USE | Needs Testing | via USE | Needs Testing | Needs Testing
  |  |   |   |   |    
filestore (pre-existing) | via USE | via USE | via USE | via STARTUP | via USE
nfs-server (pre-existing) | via USE | via USE | via USE | via STARTUP | via USE
Managed Lustre (pre-existing) | via USE| Needs Testing | via USE | Needs Testing | Needs Testing
GCS FUSE (pre-existing) | via USE | via USE | via USE | via STARTUP | via USE

- **via USE:** Client installation and mounting occur automatically when
  connected with the use field. See
  [mounting via use section](#mounting-via-use).
- **via STARTUP:** Startup scripts are provided that can be used with the
  `startup-script` module to install clients and mount. See
  [mounting via startup section](#mounting-via-startup).
- **Needs Testing:** May currently work but has not yet been fully tested.
- **Not Supported:** This feature is not supported right now.

\* only supported on CentOS 7

[filestore]: ../modules/file-system/filestore/README.md
[pre-existing-network-storage]: ../modules/file-system/pre-existing-network-storage/README.md
[managed-lustre]: ../modules/file-system/managed-lustre/README.md
[nfs-server]: ../community/modules/file-system/nfs-server/README.md
[netapp-volumes]: ../modules/file-system/netapp-volume/README.md
[netapp-storage-pool]: ../modules/file-system/netapp-storage-pool/README.md
