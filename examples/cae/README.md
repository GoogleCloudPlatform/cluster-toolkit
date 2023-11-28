# Computer Aided Engineering (CAE) Reference Architecture

The Computer Aided Engineering (CAE) [blueprint](./cae-slurm.yaml) in
this folder captures a reference architecture where the right cloud components
are assembled to optimally cater to the requirements of computationally-intensive
CAE workloads. Specifically, it is architected around Google Cloud’s VM families
that provide a high memory bandwidth and a balanced memory/flop ratio, which is
particularly useful for per-core licensed CAE software. The solution caters also
to large CAE use cases, requiring multiple nodes that are tightly-coupled via MPI.
Special high-memory shapes support even very memory-demanding workloads with up
to 16GB/core. For file IO, different Google managed high performance NFS storage
services are available. For very IO demanding workloads, third party parallel file
systems can be integrated. The scheduling of the workloads is done by a workload
manager.

## Architecture
The CAE blueprint is intended to be a starting point for more tailored explorations
or installations of specific CAE codes, as provided by ISVs separately.

This blueprint features a general setup suited for CAE applications on GCP
including:

- Google's H3 VMs, ideally suited for CAE workloads
- Google's C3-highmem VM, suited for workloads with 8GB/core requirement
- Google's Filestore NFS-based shared storage
- Google's Chrome Remote Desktop
- SLURM workload scheduler

## Getting Started
To explore the reference architecture, you should follow the these steps:

Before you start, make sure your prerequisites and dependencies are set up:
[Set up Cloud HPC Toolkit](https://cloud.google.com/hpc-toolkit/docs/setup/configure-environment).

For deploying the CAE reference blueprint follow the
[Deployment Instructions](#deployment-instructions).

For pointers on how to proceed with the installation of ISV software, please see section
[Software Installation Patterns](#software-installation-patterns).

### Deployment Stages

This blueprint has the following deployment groups:

- `setup`: Setup backbone infrastructure such as networking, file systems, &
  monitoring.
- `software_installation`(_optional_): This deployment group is a stub for
  custom software installation on the network storage before the cluster is brought up
- `cluster`: Deploys an auto-scaling cluster and remote desktop.

Having multiple deployment groups decouples the life cycle of some
infrastructure. For example a) you can tear down the cluster while leaving the
storage intact and b) you can build software before you deploy your cluster.

## Deployment Instructions

> [!WARNING]
> Installing this blueprint uses the following billable components of Google
> Cloud:
>
> - Compute Engine
> - Filestore
>
> To avoid continued billing after use closely follow the
> [teardown instructions](#teardown-instructions). To generate a cost estimate based on
> your projected usage, use the [pricing calculator](https://cloud.google.com/products/calculator).
>
> [!WARNING]
> Before attempting to execute the following instructions, it is important to
> consider your project's quota. The `cae-slurm.yaml` blueprint creates an
> autoscaling cluster that, when fully scaled up, can deploy up to 10
> `h3-standard-88` and up to 10 `c3-highmem-176` VMs.
>
> To fully scale up this cluster, the project would require quota for:
>
> - Compute Node Group
>   - 88 CPUs * 10 VMs = **880 `H3 CPUs`**
>   - 176 CPUs * 10 VMs = **1760 `C3 CPUs`**
> - Remote Desktop Group
>   - **40 `N1 CPUs`**
>   - **5 `T4 GPUs`**
> - Slurm Login & Controller VM
>   - **24 `N2 CPUs`**
> - Filestore
>   - **2x `Basic SSD`**
>   - **1x `High Scale SSD`**
>
> However, this is merely an example sizing for an instance of this reference architecture.
> Node counts and remote desktop seats can easily be adjusted in the blueprint.

1. Clone the repo

   ```bash
   git clone https://github.com/GoogleCloudPlatform/hpc-toolkit.git
   cd hpc-toolkit
   ```

1. Build the HPC Toolkit

   ```bash
   make
   ```

1. Generate the deployment folder after replacing `<project>` with the project
   id.

   ```bash
   ./ghpc create community/examples/cae-slurm.yaml -w --vars project_id=<project>
   ```

1. Deploy the `setup` group

   Call the following ghpc command to deploy the cae-slurm blueprint.

   ```bash
   ./ghpc deploy cae-slurm
   ```

   The next `ghpc` prompt will ask you to **display**, **apply**, **stop**, or
   **continue** without applying the `setup` group. Select 'apply'.

   This group will create a network and file systems to be used by the cluster.

   > [!WARNING]
   > This ghpc command will run through 2 deployment groups (3 if you populate
   > & activate the `software_installation` stage) and prompt you to apply each one.
   > If the command is cancelled or exited by accident before finishing, it can
   > be rerun to continue deploying the blueprint.

1. Deploy the `software_installation` group (_optional_).

   > [!NOTE]
   > Installation processes differ between applications. Some come as a
   > precompiled binary with all dependencies included, others may need to
   > be built from source, while others can be deployed through package
   > managers such as spack. This deployment group is intended to be used
   > if the software installation process requires substantial amount of time (e.g.
   > compilation from source). By building the software in a separate
   > deployment group, this process can be done before the cluster is
   > up, minimizing costs.
   >
   > [!NOTE]
   > By default, this deployment group is disabled in the reference design. See
   > [Software Installation Patterns](#software-installation-patterns) for more information.

   If this deployment group is used (needs to be uncomment in the blueprint first),
   you can return to the ghpc command which will ask you to **display**, **apply**,
   **stop**, or **continue** without applying the `software_installation` group.
   Select 'apply'.

1. Deploy the `cluster` group

   The next `ghpc` prompt will ask you to **display**, **apply**, **stop**, or
   **continue** without applying the `cluster` group. Select 'apply'.

   This deployment group contains the Slurm cluster and with compute partitions
   and a partition for Chrome remote desktop visualization nodes.

1. Set up Chrome Remote Desktop

   One or multiple Chrome Remote Desktop (CRD) sessions can be started dynamically
   through Slurm.

   - Follow
     [the instructions](../README.md#hpc-slurm-chromedesktopyaml--)
     for setting up the Remote Desktop.

## Teardown Instructions

> [!NOTE]
> If you created a new project for testing of the CAE solution, the easiest way to
> eliminate billing is to delete the project.

When you would like to tear down the deployment, each stage must be destroyed.
Since the `software_installation` and `cluster` depend on the network deployed
in the `setup` stage, they must be destroyed first. You can use the following
commands to destroy the deployment in this reverse order. You will be prompted
to confirm the deletion of each stage.

```bash
./ghpc destroy cae-slurm
```

> [!WARNING]
> If you do not destroy all three deployment groups then there may be continued
> associated costs.

## Software Installation Patterns

This section is intended to illustrate how software can be installed in the context
of the CAE reference solution.

Depending on the software you want to use, different installation paths may be required.

- **Installation with binary**
  Commercial-off-the-shelf CAE applications typically come with
  precompiled binaries which are provided by the ISV.

  See the tutorials for
  [Ansys Fluent](https://cloud.google.com/hpc-toolkit/docs/tutorials/ansys-fluent#install_ansys_fluent)
  and for [Siemens Simcenter STAR-CCM+](https://cloud.google.com/hpc-toolkit/docs/simcenter-star-ccm/run-workload#configure_the_vm)
  that illustrate this process.

  In general, you need to bring the binaries to your CAE cluster for which it is
  useful to use a Google Clouds Storage bucket, which is accessible from any machine using the
  gsutil command and which can be mounted in the cluster.

  As this installation process only needs to be done once and at the same time may require time,
  we recommend to do this installation in a separate deployment group before you bring up the cluster.
  The `software_installation' stage is meant to accommodate this. You can for example bring up
  a dedicated VM

  ``` {.yaml}
  - id: sw-installer-vm
    source: modules/compute/vm-instance
    use: [network1, appsfs]
    settings:
      name_prefix: sw-installer
      add_deployment_name_before_prefix: true
      threads_per_core: 2
      machine_type: c2-standard-16
  ```

  where you can follow the installation steps manually. Or using the toolkit's
  [startup-script](../../modules/scripts/startup-scripts/README.md) module, the process
  can be automated.

  Once that is completed, the software will persist on the NFS Filestore share for as long as you
  do not destroy the `setup` stage.

- **Installation from source/with package manager**
  For open source software, you may want to compile the software from scratch or use a
  package manager such as spack for the installation. This process typically takes
  a non-negligible amount of time (~hours). We therefore strongly suggest to use
  the `software_installation` stage for this purpose.

  Please see the [HCLS Blueprint](../../docs/videos/healthcare-and-life-sciences/README.md) example
  for how the `software_installation` stage can be used to use the spack package manager
  to install all dependencies for a particular version of the software, including compiling
  the software or its dependencies from source.

  Please also see the [OpenFOAM](../../docs/tutorials/openfoam/spack-openfoam.md) example
  for how this can be used to install the OpenFOAM software.

  Once that is completed, the software will persist on the NFS Filestore share for as long as you
  do not destroy the `setup` stage.
