# Electronic Design Automation (EDA) Reference Architecture

The Electronic Design Automation (EDA) blueprints in
this folder capture a reference architecture where the right cloud components
are assembled to optimally cater to the requirements of EDA workloads.

For file IO, Google Cloud NetApp Volumes NFS storage services are available.
It scales from small to high capacity and high performance and provides fan-out
caching of on-premises ONTAP systems into Google Cloud to enable hybrid cloud
architecture. The scheduling of the workloads is done by a workload
manager.

## Architecture
The EDA blueprints are intended to be a starting point for more tailored
explorations of EDA.

This blueprint features a general setup suited for EDA applications on
Google Cloud including:

- Google Compute Engine partitions
- Google Cloud NetApp Volumes NFS-based shared storage
- Slurm workload scheduler

Two example blueprints are provided.

### Blueprint [eda-all-on-cloud](eda-all-on-cloud.yaml)

This blueprint assumes that all compute and data resides in the cloud.

In the base deployment group (see [deployment stages](#deployment_stages)) it provisions a new network and multiple volumes to store your data. Adjust the volume sizes to suit your requirements before deployment. If your volumes are larger than 15 TiB, creating them as [large volumes](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/overview#large-capacity-volumes) adds performance benefits. One limitation currently is that Slurm will only use the first IP of a large volume. If you need to utilize the full performance of the 6 IP addresses a large volume provides, you can instead utilize the approach with pre-existing volumes and CloudDNS mentioned in eda-hybrid-cloud blueprint description.

The cluster deployment group deploys a managed instance group which is managed by Slurm.

When scaling down the deployment, make sure to only destroy the *compute* deployment group. If you destroy the *base* group too, all the volumes will be deleted and you will lose your data.

### Blueprint [eda-hybrid-cloud](./eda-hybrid-cloud.yaml)

This blueprint assumes you are using NetApp Volumes [FlexCache](https://docs.cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/cache-ontap-volumes/overview) to enable a [hybrid cloud EDA](https://community.netapp.com/t5/Tech-ONTAP-Blogs/NetApp-FlexCache-Enhancing-hybrid-EDA-with-Google-Cloud-NetApp-Volumes/ba-p/462768) environment.

The base deployment group (see [deployment stages](#deployment_stages)) connects to an existing network and mounts multiple volumes. This blueprint assumes you have pre-existing volumes for "tools", "libraries", "home" and "scratch". Before deployment, update `server_ip` and `remote_mount` parameters of the respective volumes in the blueprint declarations to reflect the actual IP and export path of your existing volumes. Using existing volumes also avoids the danger of being deleted accidentally when deleting the base deployment group.

The volumes used can be regular NetApp Volume [volumes](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/overview), [large volumes](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/overview#large-capacity-volumes) or [FlexCache volumes](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/cache-ontap-volumes/overview).

FlexCache offers the following features which enable bursting on-premises workloads into Google Cloud to use its powerful compute options:

- Read-writable sparse volume
- Block-level, “pull only” paradigm
- 100% consistent, coherent, current
- write-around
- LAN-like latencies after first read
- Fan-out. Use multiple caches to scale out workload

It can accelerate metadata- or throughput-heavy read workloads considerably.

FlexCache and Large Volumes offer six IP addresses per volume which all provide access to the same data. Currently Cluster Toolkit only uses one of these IPs. Support for using all 6 IPs is planned for a later release. To spread your compute nodes over all IPs today, you can use CloudDNS to create an DNS record with all 6 IPs and specify that DNS name instead of individual IPs in the blueprint. CloudDNS will return one of the 6 IPs in a round-robin fashion on lookups.

The cluster deployment group deploys a managed instance group which is managed by Slurm.

## Getting Started
To explore the reference architecture, you should follow these steps:

Before you start, make sure your prerequisites and dependencies are set up:
[Set up Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).

For deploying the EDA reference blueprint follow the
[Deployment Instructions](#deployment-instructions).

### Deployment Stages

This blueprint has the following deployment groups:

- `base`: Setup backbone infrastructure such as networking and file systems
- `software_installation`(_optional_): This deployment group is a stub for
  custom software installation on the network storage before the cluster is brought up
- `cluster`: Deploys an auto-scaling cluster

Having multiple deployment groups decouples the life cycle of some
infrastructure. For example a) you can tear down the cluster while leaving the
storage intact and b) you can build software before you deploy your cluster.

## Deployment Instructions

> [!WARNING]
> Installing this blueprint uses the following billable components of Google
> Cloud:
>
> - Compute Engine
> - NetApp Volumes
>
> To avoid continued billing after use closely follow the
> [teardown instructions](#teardown-instructions). To generate a cost estimate based on
> your projected usage, use the [pricing calculator](https://cloud.google.com/products/calculator).
>
> [!WARNING]
> Before attempting to execute the following instructions, it is important to
> consider your project's quota. The blueprints create an
> autoscaling cluster that, when fully scaled up, can deploy many powerful VMs.
>
> This is merely an example for an instance of this reference architecture.
> Node counts can easily be adjusted in the blueprint.

1. Clone the repo

   ```bash
   git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
   cd cluster-toolkit
   ```

1. Build the Cluster Toolkit

   ```bash
   make
   ```

1. Change parameters in your blueprint file to reflect your requirements. Examples are VPC names for existing networks, H4D instance group node limits or export paths of existing NFS volumes.

1. Generate the deployment folder after replacing `<blueprint>` with the name of the blueprint (`eda-all-on-cloud` or `eda-hybrid-cloud`) and `<project_id>`, `region` and `zone` with your project details.

   ```bash
   ./gcluster create community/examples/eda/<blueprint>.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}" --vars region=us-central1 --vars zone=us-central1-a
   ```

1. Deploy the `base` group

   Call the following gcluster command to deploy the blueprint.

   ```bash
   ./gcluster deploy CLUSTER-NAME
   ```

   Replace `CLUSTER-NAME` with the deployment_name (`eda-all-on-cloud` or
   `eda-hybrid-cloud`) used in the blueprint vars block.

   The next `gcluster` prompt will ask you to **display**, **apply**, **stop**, or
   **continue** without applying the `base` group. Select 'apply'.

   This group will create a network and file systems to be used by the cluster.

   > [!WARNING]
   > This gcluster command will run through 2 deployment groups (3 if you populate
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

   If this deployment group is used (needs to be uncommented in the blueprint first),
   you can return to the gcluster command which will ask you to **display**, **apply**,
   **stop**, or **continue** without applying the `software_installation` group.
   Select 'apply'.

1. Deploy the `cluster` group

   The next `gcluster` prompt will ask you to **display**, **apply**, **stop**, or
   **continue** without applying the `cluster` group. Select 'apply'.

   This deployment group contains the Slurm cluster and compute partitions.

## Teardown Instructions

> [!NOTE]
> If you created a new project for testing of the EDA solution, the easiest way to
> eliminate billing is to delete the project.

When you would like to tear down the deployment, each stage must be destroyed.
Since the `software_installation` and `cluster` depend on the network deployed
in the `base` stage, they must be destroyed first. You can use the following
commands to destroy the deployment in this reverse order. You will be prompted
to confirm the deletion of each stage.

```bash
./gcluster destroy CLUSTER-NAME
```

Replace `CLUSTER-NAME` with the deployment_name (`eda-all-on-cloud` or
`eda-hybrid-cloud`) used in the blueprint vars block.

> [!WARNING]
> If you do not destroy all three deployment groups then there may be continued
> associated costs.

## Software Installation Patterns

This section is intended to illustrate how software can be installed in the context
of the EDA reference solution.

Depending on the software you want to use, different installation paths may be required.

- **Installation with binary**
  Commercial-off-the-shelf applications typically come with precompiled binaries which
  are provided by the ISV. If you do not share them using the toolsfs or libraryfs shares,
  you can install software using the following method.

  In general, you need to bring the binaries to your EDA cluster for which it is
  useful to use a Google Cloud Storage bucket, which is accessible from any machine using the
  gsutil command and which can be mounted in the cluster.

  As this installation process only needs to be done once and at the same time may require time,
  we recommend to do this installation in a separate deployment group before you bring up the cluster.
  The `software_installation` stage is meant to accommodate this. You can for example bring up
  a dedicated VM

  ``` {.yaml}
  - id: sw-installer-vm
    source: modules/compute/vm-instance
    use: [network1, toolsfs]
    settings:
      name_prefix: sw-installer
      add_deployment_name_before_prefix: true
      threads_per_core: 2
      machine_type: c2-standard-16
  ```

  where you can follow the installation steps manually. Or using the toolkit's
  [startup-script](../../modules/scripts/startup-scripts/README.md) module, the process
  can be automated.

  Once that is completed, the software will persist on the NetApp Volumes share for as long as you
  do not destroy the `base` stage.

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

  Once that is completed, the software will persist on the NetApp Volumes share for as long as you
  do not destroy the `base` stage.
