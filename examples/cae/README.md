# Computer Aided Engineering (CAE) Reference Architecture

The Computer Aided Engineering (CAE) [blueprint](./cae-slurm.yaml) and accompanying
[deployment configuration](./cae-slurm-deployment.yaml) in
this folder capture a reference architecture where the right cloud components
are assembled to optimally cater to the requirements of computationally-intensive
CAE workloads. Specifically, it is architected around Google Cloud’s VM families
that provide a high memory bandwidth and a balanced memory/flop ratio, which is
particularly useful for per-core licensed CAE software. The solution caters also
to large CAE use cases, requiring multiple nodes that are tightly-coupled via MPI
over Cloud RDMA. Special high-memory shapes support even very memory-demanding
workloads with up to 16GB/core, while GPU partitions support GPU-accelerated CAE
solvers and visualization workloads. For file IO, Google managed high-performance
NFS storage services (Filestore) are used for `/home` and `/apps`, and Google Cloud
Managed Lustre provides a high-throughput parallel file system for `/scratch`.
The scheduling of the workloads is done by the Slurm workload manager.

## Architecture

The CAE blueprint is intended to be a starting point for more tailored explorations
or installations of specific CAE codes, as provided by ISVs separately.

This blueprint features a general setup suited for CAE applications on GCP
including:

- Google's H4D VMs (`h4d-highmem-192-lssd`) with Cloud RDMA and Local SSD (`/mnt/lssd`), ideally suited for tightly-coupled CAE workloads (`h4d` default partition)
- Google's C4D-highmem VMs (`c4d-highmem-192`) with Tier 1 networking, suited for workloads with 16GB/core requirement (`highmem` partition)
- Google's G4 GPU VMs (`g4-standard-48` by default, or `g4-standard-384` with Dual NIC) with NVIDIA DCGM monitoring enabled, suited for GPU-accelerated CAE and visualization workloads (`g4` partition)
- Google's Filestore NFS-based shared storage for `/home` and `/apps`
- Google's Managed Lustre parallel file system for `/scratch`
- Google's Chrome Remote Desktop with NVIDIA T4 GPU and shared `/home` and `/apps` mounts
- SLURM workload scheduler with OS Admin Login password-free `sudo` and pre-job iRDMA health checks on H4D nodes

## Getting Started

To explore the reference architecture, you should follow these steps:

Before you start, make sure your prerequisites and dependencies are set up:
[Set up Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).

For deploying the CAE reference blueprint follow the
[Deployment Instructions](#deployment-instructions).

For pointers on how to proceed with the installation of ISV software, please see section
[Software Installation Patterns](#software-installation-patterns).

### Deployment Stages

This blueprint has the following deployment groups:

- `enable_apis`: Enables required Google Cloud APIs (Compute Engine, Filestore,
  Managed Lustre, Service Networking, etc.).
- `setup`: Sets up backbone infrastructure such as the primary VPC network,
  Private Service Access (required for Managed Lustre), the secondary Cloud RDMA
  (IRDMA) network for H4D nodes, persistent NFS Filestore shares (`/home` and `/apps`),
  and monitoring dashboard.
- `software_installation` (_optional_): This deployment group is a stub for
  custom software installation on the network storage before the cluster is brought up.
- `cluster`: Deploys the Managed Lustre `/scratch` file system, Chrome Remote Desktop,
  and the auto-scaling Slurm cluster (`h4d`, `highmem`, and `g4` partitions).

Having multiple deployment groups decouples the life cycle of some
infrastructure. For example a) you can tear down the cluster and Managed Lustre
scratch storage while leaving the persistent `/home` and `/apps` NFS storage intact
and b) you can build software before you deploy your cluster.

## Configuration

You can customize your deployment settings in [`cae-slurm-deployment.yaml`](./cae-slurm-deployment.yaml):

- **Project & Location**: Set `project_id`, `deployment_name` (defaults to `cae-slurm-solutions`), `region`, and `zone` (ensure the selected zone supports H4D, C4D, G4, and Managed Lustre).
- **H4D & Cloud RDMA Settings**: Configure `rdma_net_range`, `h4d_cluster_size` (static node count), and the provisioning model (`h4d_reservation_name`, `h4d_dws_flex_enabled`, or `h4d_enable_spot_vm`).
- **G4 GPU Settings**: Configure `g4_cluster_size`, `g4_machine_type` (`g4-standard-48` or `g4-standard-384`), `g4_disk_size_gb`, and the provisioning model (`g4_reservation_name`, `g4_enable_spot_vm`, or `g4_dws_flex_enabled`). To use Dual NIC with `g4-standard-384`, uncomment `net1_range` in the deployment file as well as the `net1` module and `additional_networks` setting in [`cae-slurm.yaml`](./cae-slurm.yaml).
- **Managed Lustre Settings**: Configure `lustre_instance_id`, `lustre_size_gib` (default `9000`), and `per_unit_storage_throughput` (default `1000`). Refer to [Managed Lustre performance tiers](https://cloud.google.com/managed-lustre/docs/create-instance#performance-tiers) for valid size and throughput combinations.

## Deployment Instructions

> [!WARNING]
> Installing this blueprint uses the following billable components of Google
> Cloud:
>
> - Compute Engine
> - Filestore
> - Managed Lustre
>
> To avoid continued billing after use closely follow the
> [teardown instructions](#teardown-instructions). To generate a cost estimate based on
> your projected usage, use the [pricing calculator](https://cloud.google.com/products/calculator).

### Quotas and Permissions

> [!WARNING]
> Before attempting to execute the following instructions, it is important to
> consider your project's IAM permissions and quota.
>
> **IAM Permissions**: Private Service Access (required for Managed Lustre)
> requires the `compute.networkAdmin` (or `servicenetworking.networksAdmin`) role,
> which is included in the `Owner` role but not the `Editor` role.
>
> **Quotas**: The `cae-slurm.yaml` blueprint creates an autoscaling cluster that,
> when fully scaled up with default settings, can deploy up to 10
> `h4d-highmem-192-lssd`, up to 10 `c4d-highmem-192`, and up to 2 `g4-standard-48` VMs.
>
> To fully scale up this cluster, the project would require quota for:
>
> - Compute Node Groups
>   - 192 CPUs * 10 VMs = **1920 `H4D CPUs`** (plus Local SSD and Hyperdisk Balanced)
>   - 192 CPUs * 10 VMs = **1920 `C4D CPUs`**
>   - 48 CPUs * 2 VMs = **96 `G4 CPUs`** and **2 `NVIDIA RTX PRO 6000 GPUs`**
> - Remote Desktop Instance
>   - **8 `N1 CPUs`** (`1x n1-standard-8` by default)
>   - **1 `T4 GPU`** (`nvidia-tesla-t4-vws`)
> - Slurm Login & Controller VMs
>   - **24 `N2 CPUs`** (`n2-standard-8` login and `n2-standard-16` controller)
> - Filestore
>   - **2x `Basic SSD`** (`2560 GB` each, for `/home` and `/apps`)
> - Managed Lustre
>   - **1x `Managed Lustre`** (`9000 GiB` at `1000 MB/s/TiB` for `/scratch`)
>
> However, this is merely an example sizing for an instance of this reference architecture.
> Node counts, provisioning models (Reservations, DWS Flex-Start, Spot VMs), and remote desktop
> settings can easily be adjusted in the blueprint and deployment files.

### Step-by-Step Deployment

1. From the root of the [`cluster-toolkit`](https://github.com/GoogleCloudPlatform/cluster-toolkit) repository, build `./gcluster` (if not already built):

   ```bash
   test -x ./gcluster || make
   ```

1. Update `project_id` (and any other desired variables) in
   `examples/cae/cae-slurm-deployment.yaml`, then generate the deployment folder:

   ```bash
   ./gcluster create examples/cae/cae-slurm.yaml -d examples/cae/cae-slurm-deployment.yaml -w
   ```

   *(Alternatively, you can generate the deployment folder directly without the
   deployment file by running `./gcluster create examples/cae/cae-slurm.yaml -w --vars project_id=<project>`,
   which uses the default deployment name `cae-slurm-v6`.)*

1. Deploy the `enable_apis` and `setup` groups

   Call the following `gcluster` command to deploy the blueprint (using `cae-slurm-solutions`
   from `cae-slurm-deployment.yaml`, or `cae-slurm-v6` if you did not use the deployment file):

   ```bash
   ./gcluster deploy cae-slurm-solutions
   ```

   For each deployment group (`enable_apis`, followed by `setup`), `gcluster`
   will prompt you to **display**, **apply**, **stop**, or **continue** without
   applying. Select **apply** for each.

   The `enable_apis` group enables the required GCP service APIs, and the `setup`
   group creates the primary VPC network, Private Service Access, secondary Cloud
   RDMA network, `/home` and `/apps` Filestore instances, and monitoring dashboard.

   > [!WARNING]
   > This `gcluster` command will run through 3 deployment groups (4 if you populate
   > & activate the `software_installation` stage) and prompt you to apply each one.
   > If the command is cancelled or exited by accident before finishing, it can
   > be rerun to continue deploying the blueprint.

1. Deploy the `software_installation` group (_optional_)

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

   If this deployment group is used (needs to be uncommented in the blueprint first),
   you can return to the `gcluster` command which will ask you to **display**, **apply**,
   **stop**, or **continue** without applying the `software_installation` group.
   Select **apply**.

1. Deploy the `cluster` group

   The next `gcluster` prompt will ask you to **display**, **apply**, **stop**, or
   **continue** without applying the `cluster` group. Select **apply**.

   This deployment group provisions the Managed Lustre `/scratch` file system,
   the Chrome Remote Desktop visualization node, and the Slurm cluster with
   `h4d`, `highmem` (C4D), and `g4` (GPU) compute partitions.

1. Set up Chrome Remote Desktop

   A Chrome Remote Desktop (CRD) visualization instance with `/home` and `/apps`
   mounted is provisioned as part of the `cluster` group.

   - Follow the
     [chrome-remote-desktop module instructions](../../community/modules/remote-desktop/chrome-remote-desktop/README.md)
     for configuring and connecting to the Remote Desktop.

## Teardown Instructions

> [!NOTE]
> If you created a new project for testing of the CAE solution, the easiest way to
> eliminate billing is to delete the project.

When you would like to tear down the deployment, each stage must be destroyed.
Since the `software_installation` and `cluster` groups depend on the network deployed
in the `setup` stage, they must be destroyed first. You can use the following
command to destroy the deployment in this reverse order. You will be prompted
to confirm the deletion of each stage.

```bash
./gcluster destroy cae-slurm-solutions
```

> [!WARNING]
> If you do not destroy all deployment groups then there may be continued
> associated costs.

## Software Installation Patterns

This section is intended to illustrate how software can be installed in the context
of the CAE reference solution.

Depending on the software you want to use, different installation paths may be required.

- **Installation with binary**
  Commercial-off-the-shelf CAE applications typically come with
  precompiled binaries which are provided by the ISV.

  See the tutorials for
  [Ansys Fluent](https://cloud.google.com/cluster-toolkit/docs/tutorials/ansys-fluent#install_ansys_fluent)
  and for [Siemens Simcenter STAR-CCM+](https://cloud.google.com/cluster-toolkit/docs/simcenter-star-ccm/run-workload#configure_the_vm)
  that illustrate this process.

  In general, you need to bring the binaries to your CAE cluster for which it is
  useful to use a Google Cloud Storage bucket, which is accessible from any machine using the
  `gcloud storage` commands or mounted directly in the cluster.

  As this installation process only needs to be done once and at the same time may require time,
  we recommend to do this installation in a separate deployment group before you bring up the cluster.
  The `software_installation` stage is meant to accommodate this. You can for example bring up
  a dedicated VM

  ``` {.yaml}
  - id: sw-installer-vm
    source: modules/compute/vm-instance
    use: [network, appsfs]
    settings:
      name_prefix: sw-installer
      add_deployment_name_before_prefix: true
      threads_per_core: 2
      machine_type: c2-standard-16
  ```

  where you can follow the installation steps manually. Or using the toolkit's
  [startup-script](../../modules/scripts/startup-script/README.md) module, the process
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

  Please also see the [GROMACS](../../docs/tutorials/gromacs/spack-gromacs.md) example
  for how this can be used to install scientific software using Spack.

  Once that is completed, the software will persist on the NFS Filestore share for as long as you
  do not destroy the `setup` stage.
