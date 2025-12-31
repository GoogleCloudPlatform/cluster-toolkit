## Description

This module provides scripts for client installation and mounting [WEKA]
filesystems. Client supports both UDP and DPDK modes and allows customization of
mount parameters using Compute VM instance metadata.

For deploying Weka cluster please consult [WEKA installation on GCP].

[WEKA]: https://www.weka.io/
[WEKA installation on GCP]: https://docs.weka.io/planning-and-installation/weka-installation-on-gcp

## Prerequisites

* up and running Weka cluster
* running on a [supported OS](https://docs.weka.io/planning-and-installation/prerequisites-and-compatibility#operating-system)
* [open firewall](https://docs.weka.io/planning-and-installation/prerequisites-and-compatibility#required-ports)
  between WEKA backend servers and clients
* VPC peering configuration:
  * if clients share VPCs created for WEKA cluster, no additional configuration
    is necessary
  * if dedicated VPCs are in use for clients, then WEKA VPCs needs to be peered
    with VPCs that are used as:
    * primary interface on client
    * interfaces dedicated for DPDK client
  * if dedicated VPCs are in use for clients, then those VPCs needs to be peered
    with each other

## Mounting
This example creates mount scripts that will mount `default` filesystem from
`10.0.0.3` WEKA backend:

```yaml
      - id: wekafs
        source: community/modules/file-system/weka-client
        settings:
          local_mount: /scratch
          server_ip: 10.0.0.3
          remote_mount: default

    - id: mount-at-startup
      source: modules/scripts/startup-script
      settings:
        runners: $(wekafs.runners)
```

If you need to add mount script along other runners, remember to add all 4
runners provided by this script as shown in this example:

```yaml
    - id: mount-at-startup
      source: modules/scripts/startup-script
      settings:
      runners:
        - $(wekafs.client_install_runner)
        - $(wekafs.mount_runner)
        - type: shell
          content: |
            #!/bin/bash

            echo Sample
          destination: sample-script.sh
```

To use the client within Slurm partition, with DPDK, remember to set additional
networks, and configure metadata. In this example, all four additional interfaces
are dedicated to WEKA DPDK

```yaml
  - id: c2_60_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use:
      - network
      - mount-at-startup # as defined in previous examples
    settings:
      bandwidth_tier: virtio_enabled # Weka requires VirtIO, from WEKA 4.4.1, DPDK is also supported on gVNIC
      additional_networks:
        - subnetwork: weka-client-1
          nic_type: VIRTIO_NET
        - subnetwork: weka-client-2
          nic_type: VIRTIO_NET
        - subnetwork: weka-client-3
          nic_type: VIRTIO_NET
        - subnetwork: weka-client-4
          nic_type: VIRTIO_NET
      machine_type: c2-standard-60
      metadata:
        weka-data_interfaces: 1,2,3,4 # allocate interfaces 1, 2, 3 and 4 to DPDK
        weka-mode: dpdk
        weka-options: num_cores=4,dpdk_base_memory_mb=16
      node_conf:
        # From https://docs.weka.io/planning-and-installation/bare-metal/planning-a-weka-system-installation
        # do not set RealMem as this is set automatically by Cluster Toolkit
        CoreSpecCount: 4
        MemSpecLimit: 5120
```

Due to the fact, that client installation takes ~6-7 minutes, if you use WEKA together with Slurm and do not bundle
client in the instance image, you may need to increase the timeout for startups scripts.

```yaml
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    settings:
      compute_startup_scripts_timeout: 600
      login_startup_scripts_timeout: 600
    ...
  - id: compute_partition
    source: community/modules/compute/schedmd-slurm-gcp-v6-partition
    settings:
      resume_timeout: 600
      ...
```

## Supported VM metadata options
Client scripts do support following metadata keys:
* `weka-mode` - one of `udp` or `dpdk`. Defaults to `udp`. Sets client mode.
* `weka-data_interfaces` - comma separated list of interface identifiers,
  specifying which interfaces are dedicated for data plane. Set to `1` to
  dedicate second interface of instance for WEKA DPDK. Set to `2,5` to dedicate
  third and sixth interface of instance for WEKA DPDK.
* `weka-mgmt_interface` - identifier of management interface, defaults to `0`,
  which means to use primary interface as management interface.
* `weka-options` - additional [mount command options](https://docs.weka.io/weka-filesystems-and-object-stores/mounting-filesystems#mount-command-options)
  to pass to `mount` command

## Adding client to the OS image
To save time during the mount command install and precompile DPDK drivers in the
OS image. Following scripts compiles DPDK driver for currently running kernel.

```shell
#!/bin/bash

set -e -o pipefail

echo Downloading and installing Weka client
curl --max-time 10 "{{ weka backend endpoint }}/dist/v1/install" | sh
WEKA_VERSION=$(weka -v | sed -e 's/^[^0-9]*//')
echo Installing Weka version: ${WEKA_VERSION}
weka version get "${WEKA_VERSION}"
weka version set "${WEKA_VERSION}"
# run setup for the second time, if it fails for the first time
weka local setup weka || weka local setup weka
weka version prepare "${WEKA_VERSION}"
weka local stop
weka local rm -f --all
```

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
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | The mount point where the contents of the device may be accessed after mounting. | `string` | `"/mnt"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Mount options for filesystem shared by all clients. | `string` | `""` | no |
| <a name="input_remote_mount"></a> [remote\_mount](#input\_remote\_mount) | Weka filesystem name. | `string` | n/a | yes |
| <a name="input_server_ip"></a> [server\_ip](#input\_server\_ip) | Weka backend IP address used for bootstrapping. | `string` | `""` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_client_install_runner"></a> [client\_install\_runner](#output\_client\_install\_runner) | Ansible runner that performs client installation needed to use file system. |
| <a name="output_mount_runner"></a> [mount\_runner](#output\_mount\_runner) | Ansible runner that mounts the file system. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
