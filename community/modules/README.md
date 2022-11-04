# Community Modules

To learn more about using and writing HPC toolkit modules, see the [core
module documentation](../../modules/README.md).

## Compute

* [**SchedMD-slurm-on-gcp-partition**](compute/SchedMD-slurm-on-gcp-partition/README.md):
  Creates a Slurm partition that can be used by the
  SchedMD-slurm_on_gcp_controller.
* [**schedmd-slurm-gcp-v5-partition**](compute/schedmd-slurm-gcp-v5-partition/README.md):
  Creates a Slurm partition that can be used by the
  [schedmd-slurm-gcp-v5-controller] module.
* [**schedmd-slurm-gcp-v5-node-group**](compute/schedmd-slurm-gcp-v5-node-group/README.md):
  Defines a node group that can be used as input to the
  [schedmd-slurm-gcp-v5-partition] module.
* [**pbspro-execution**](compute/pbspro-execution/README.md):
  Creates execution hosts for use in a PBS Professional cluster.

[schedmd-slurm-gcp-v5-controller]: scheduler/schedmd-slurm-gcp-v5-controller/README.md
[schedmd-slurm-gcp-v5-partition]: compute/schedmd-slurm-gcp-v5-partition/README.md

## Database

*
  [**slurm-cloudsql-federation**](database/slurm-cloudsql-federation/README.md):
  Creates a [Google SQL Instance](https://cloud.google.com/sql/) meant to be
  integrated with a
  [slurm controller](./third-pary/scheduler/SchedMD-slurm-on-gcp-controller/README.md).

## File System

* [**nfs-server**](file-system/nfs-server/README.md): Creates a VM instance and
  configures an NFS server that can be mounted by other VM instances.

* [**DDN-EXAScaler**](third-party/file-system/DDN-EXAScaler/README.md): Creates
  a [DDN EXAscaler lustre](<https://www.ddn.com/partners/google-cloud-platform/>)
  file system. This module has
  [license costs](https://console.developers.google.com/marketplace/product/ddnstorage/exascaler-cloud).

## Project

* [**new-project**](project/new-project/README.md): Creates a Google Cloud Projects

* [**service-account**](project/service-account/README.md): Creates [service
  accounts](https://cloud.google.com/iam/docs/service-accounts) for a GCP project.

* [**service-enablement**](project/service-enablement/README.md): Allows
  enabling various APIs for a Google Cloud Project

## Scripts

* [**omnia-install**](scripts/omnia-install/README.md): Installs SLURM via omnia
  onto a cluster of compute VMs

* [**pbspro-preinstall**](scripts/pbspro-preinstall/README.md): Creates a
  Cloud Storage bucket in which to save PBS Professional RPM packages for use
  by PBS clusters.

* [**pbspro-install**](scripts/pbspro-install/README.md): Creates a
  Toolkit runner to install [PBS Professional][pbspro] from RPM packages.

* [**pbspro-qmgr**](scripts/pbspro-qmgr/README.md): Creates a
  Toolkit runner to run common `qmgr` commands when configuring a PBS
  Professional cluster.

* [**spack-install**](scripts/spack-install/README.md): Creates a startup script
  to install spack on an instance or the slurm controller

* [**wait-for-startup**](scripts/wait-for-startup/README.md): Waits for
  successful completion of a startup script on a compute VM

## Scheduler

* [**SchedMD-slurm-on-gcp-controller**](scheduler/SchedMD-slurm-on-gcp-controller/README.md):
  Creates a Slurm controller node using
  [slurm-gcp](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/controller)

* [**SchedMD-slurm-on-gcp-login-node**](scheduler/SchedMD-slurm-on-gcp-login-node/README.md):
  Creates a Slurm login node using
  [slurm-gcp](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/login)

* [**schedmd-slurm-gcp-v5-login**](scheduler/schedmd-slurm-gcp-v5-login/README.md):
  Creates a Slurm login node using [slurm-gcp] version 5.

* [**schedmd-slurm-gcp-v5-controller**](scheduler/schedmd-slurm-gcp-v5-controller/README.md):
  Creates a Slurm controller using [slurm-gcp] version 5.

* [**schedmd-slurm-gcp-v5-hybrid**](scheduler/schedmd-slurm-gcp-v5-hybrid/README.md):
  Creates configurations for hybrid partitions that can be used with an
  on-premise Slurm cluster. This module uses the
  [slurm-controller-hybrid](https://github.com/SchedMD/slurm-gcp/tree/v5.1.0/terraform/slurm_cluster/modules/slurm_controller_hybrid)
  from the slurm-gcp project.

* [**pbspro-client**](scheduler/pbspro-client/README.md):
  Creates a client host for submitting jobs to a PBS Professional cluster.

* [**pbspro-server**](scheduler/pbspro-server/README.md):
  Creates a server host for operating a PBS Professional cluster.

[slurm-gcp]: https://github.com/SchedMD/slurm-gcp/tree/v5.1.0
