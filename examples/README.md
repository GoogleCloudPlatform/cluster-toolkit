# Example Blueprints

This directory contains a set of example blueprint files that can be fed into
gHPC to create a deployment.

<!-- TOC generated with some manual tweaking of the following command output:
md_toc github examples/README.md | sed -e "s/\s-\s/ * /"
-->
<!-- TOC -->

* [Instructions](#instructions)
  * [(Optional) Setting up a remote terraform state](#optional-setting-up-a-remote-terraform-state)
* [Blueprint Descriptions](#blueprint-descriptions)
  * [hpc-cluster-small.yaml](#hpc-cluster-smallyaml-) ![core-badge]
  * [hpc-cluster-high-io.yaml](#hpc-cluster-high-ioyaml-) ![core-badge]
  * [image-builder.yaml](#image-builderyaml-) ![core-badge]
  * [cloud-batch.yaml](#cloud-batchyaml-) ![core-badge]
  * [batch-mpi.yaml](#batch-mpiyaml-) ![core-badge]
  * [lustre.yaml](#lustreyaml-) ![core-badge]
  * [slurm-gcp-v5-hpc-centos7.yaml](#slurm-gcp-v5-hpc-centos7yaml-) ![community-badge]
  * [slurm-gcp-v5-ubuntu2004.yaml](#slurm-gcp-v5-ubuntu2004yaml-) ![community-badge]
  * [slurm-gcp-v5-high-io.yaml](#slurm-gcp-v5-high-ioyaml-) ![community-badge]
  * [hpc-cluster-intel-select.yaml](#hpc-cluster-intel-selectyaml-) ![community-badge]
  * [daos-cluster.yaml](#daos-clusteryaml-) ![community-badge]
  * [daos-slurm.yaml](#daos-slurmyaml-) ![community-badge]
  * [hpc-cluster-amd-slurmv5.yaml](#hpc-cluster-amd-slurmv5yaml-) ![community-badge]
  * [quantum-circuit-simulator.yaml](#quantum-circuit-simulatoryaml-) ![community-badge]
  * [spack-gromacs.yaml](#spack-gromacsyaml--) ![community-badge] ![experimental-badge]
  * [omnia-cluster.yaml](#omnia-clusteryaml--) ![community-badge] ![experimental-badge]
  * [hpc-cluster-small-sharedvpc.yaml](#hpc-cluster-small-sharedvpcyaml--) ![community-badge] ![experimental-badge]
  * [hpc-cluster-localssd.yaml](#hpc-cluster-localssdyaml--) ![community-badge] ![experimental-badge]
  * [htcondor-pool.yaml](#htcondor-poolyaml--) ![community-badge] ![experimental-badge]
  * [gke.yaml](#gkeyaml--) ![community-badge] ![experimental-badge]
  * [starccm-tutorial.yaml](#starccm-tutorialyaml--) ![community-badge] ![experimental-badge]
  * [fluent-tutorial.yaml](#fluent-tutorialyaml--) ![community-badge] ![experimental-badge]
* [Blueprint Schema](#blueprint-schema)
* [Writing an HPC Blueprint](#writing-an-hpc-blueprint)
  * [Blueprint Boilerplate](#blueprint-boilerplate)
  * [Top Level Parameters](#top-level-parameters)
  * [Deployment Variables](#deployment-variables)
  * [Deployment Groups](#deployment-groups)
* [Variables](#variables)
  * [Blueprint Variables](#blueprint-variables)
  * [Literal Variables](#literal-variables)
  * [Escape Variables](#escape-variables)

## Instructions

Ensure `project_id`, `zone`, and `region` deployment variables are set correctly
under `vars` before using an example blueprint.

> **_NOTE:_** Deployment variables defined under `vars` are automatically passed
> to modules if the modules have an input that matches the variable name.

### (Optional) Setting up a remote terraform state

The following block will configure terraform to point to an existing GCS bucket
to store and manage the terraform state. Add your own bucket name in place of
`<<BUCKET_NAME>>` and (optionally) a service account in place of
`<<SERVICE_ACCOUNT>>` in the configuration. If not set, the terraform state will
be stored locally within the generated deployment directory.

Add this block to the top-level of your blueprint:

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <<BUCKET_NAME>>
    impersonate_service_account: <<SERVICE_ACCOUNT>>
```

You can set the configuration using the CLI in the `create` and `expand`
subcommands as well:

```shell
./ghpc create examples/hpc-cluster-small.yaml \
  --vars "project_id=${GOOGLE_CLOUD_PROJECT}" \
  --backend-config "bucket=${GCS_BUCKET}"
```

> **_NOTE:_** The `--backend-config` argument supports comma-separated list of
> name=value variables to set Terraform Backend configuration in blueprints.
> This feature only supports variables of string type. If you set configuration
> in both the blueprint and CLI, the tool uses values at CLI. "gcs" is set as
> type by default.

## Blueprint Descriptions

[core-badge]: https://img.shields.io/badge/-core-blue?style=plastic
[community-badge]: https://img.shields.io/badge/-community-%23b8def4?style=plastic
[stable-badge]: https://img.shields.io/badge/-stable-lightgrey?style=plastic
[experimental-badge]: https://img.shields.io/badge/-experimental-%23febfa2?style=plastic

The example blueprints listed below labeled with the core badge
(![core-badge]) are located in this folder and are developed and tested by the
HPC Toolkit team directly.

The community blueprints are contributed by the community (including the HPC
Toolkit team, partners, etc.) and are labeled with the community badge
(![community-badge]). The community blueprints are located in the
[community folder](../community/examples/).

Blueprints that are still in development and less stable are also labeled with
the experimental badge (![experimental-badge]).

### [hpc-cluster-small.yaml] ![core-badge]

Creates a basic auto-scaling Slurm cluster with mostly default settings. The
blueprint also creates a new VPC network, and a filestore instance mounted to
`/home`.

There are 2 partitions in this example: `debug` and `compute`. The `debug`
partition uses `n2-standard-2` VMs, which should work out of the box without
needing to request additional quota. The purpose of the `debug` partition is to
make sure that first time users are not immediately blocked by quota
limitations.

[hpc-cluster-small.yaml]: ./hpc-cluster-small.yaml

#### Compute Partition

There is a `compute` partition that achieves higher performance. Any
performance analysis should be done on the `compute` partition. By default it
uses `c2-standard-60` VMs with placement groups enabled. You may need to request
additional quota for `C2 CPUs` in the region you are deploying in. You can
select the compute partition using the `-p compute` argument when running `srun`.

#### Quota Requirements for hpc-cluster-small.yaml

For this example the following is needed in the selected region:

* Cloud Filestore API: Basic HDD (Standard) capacity (GB): **1,024 GB**
* Compute Engine API: Persistent Disk SSD (GB): **~50 GB**
* Compute Engine API: Persistent Disk Standard (GB): **~20 GB static + 20
  GB/node** up to 500 GB
* Compute Engine API: N2 CPUs: **10**
* Compute Engine API: C2 CPUs: **4** for controller node and **60/node** active
  in `compute` partition up to 1,204
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for `compute` partition_

### [hpc-cluster-high-io.yaml] ![core-badge]

Creates a Slurm cluster with tiered file systems for higher performance. It
connects to the default VPC of the project and creates two partitions and a
login node.

File systems:

* The homefs mounted at `/home` is a default "BASIC_HDD" tier filestore with
  1 TiB of capacity
* The projectsfs is mounted at `/projects` and is a high scale SSD filestore
  instance with 10TiB of capacity.
* The scratchfs is mounted at `/scratch` and is a
  [DDN Exascaler Lustre](../community/modules/file-system/DDN-EXAScaler/README.md)
  file system designed for high IO performance. The capacity is ~10TiB.

> **Warning**: The DDN Exascaler Lustre file system has a license cost as
> described in the pricing section of the
> [DDN EXAScaler Cloud Marketplace Solution](https://console.developers.google.com/marketplace/product/ddnstorage/).

There are two partitions in this example: `low_cost` and `compute`. The
`low_cost` partition uses `n2-standard-4` VMs. This partition can be used for
debugging and workloads that do not require high performance.

Similar to the small example, there is a
[compute partition](#compute-partition) that should be used for any performance
analysis.

#### Quota Requirements for hpc-cluster-high-io.yaml

For this example the following is needed in the selected region:

* Cloud Filestore API: Basic HDD (Standard) capacity (GB) per region: **1,024 GB**
* Cloud Filestore API: High Scale SSD capacity (GB) per region: **10,240 GiB** - _min
  quota request is 61,440 GiB_
* Compute Engine API: Persistent Disk SSD (GB): **~14,050 GB**
* Compute Engine API: Persistent Disk Standard (GB): **~396 GB static + 20
  GB/node** up to 4596 GB
* Compute Engine API: N2 CPUs: **158**
* Compute Engine API: C2 CPUs: **8** for controller node and **60/node** active
  in `compute` partition up to 12,008
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for `compute` partition_

[hpc-cluster-high-io.yaml]: ./hpc-cluster-high-io.yaml

### [image-builder.yaml] ![core-badge]

This Blueprint uses the [Packer template module][pkr] to create custom VM images
by applying software and configurations to existing images. This example takes
the following steps:

1. Creates a network with outbound internet access in which to build the image (see
[Custom Network](#custom-network-deployment-group-1)).
2. Creates a script that will be used to customize the image (see
[Toolkit Runners](#toolkit-runners-deployment-group-1)).
3. Builds a custom Slurm image by executing the script on a standard Slurm image
(see [Packer Template](#packer-template-deployment-group-2)).
4. Deploys a Slurm cluster using the custom image (see
[Slurm Cluster Based on Custom Image](#slurm-cluster-based-on-custom-image-deployment-group-3)).

Create the deployment folder from the blueprint:

```shell
./ghpc create examples/image-builder.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}"
```

Follow the on-screen commands that direct you to execute `terraform`, `packer`,
and `ghpc` using the `export-outputs` / `import-inputs` sub-commands.
The `export-outputs` / `import-inputs` sub-commands propagate dynamically
created values from early steps in the build process to later steps. For
example, the network is created in the first deployment group and its name
must be supplied to both the Packer and Slurm cluster deployment groups. These
sub-commands automate steps that might otherwise require manual copying.

When you are done, clean up the resources in reverse order of creation

```shell
terraform -chdir=image-builder-001/cluster destroy --auto-approve
terraform -chdir=image-builder-001/builder-env destroy --auto-approve
```

Finally, browse to the [Cloud Console][console-images] to delete your custom
image. It will be named beginning with `my-slurm-image` followed by a date and
timestamp for uniqueness.

[console-images]: https://console.cloud.google.com/compute/images

#### Why use a custom image?

Using a custom VM image can be more scalable and reliable than installing
software using boot-time startup scripts because:

* it avoids reliance on continued availability of package repositories
* VMs will join an HPC cluster and execute workloads more rapidly due to reduced
  boot-time configuration
* machines are guaranteed to boot with a static software configuration chosen
  when the custom image was created. No potential for some machines to have
  different software versions installed due to `apt`/`yum`/`pip` installations
  executed after remote repositories have been updated.

[hpcimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm
[pkr]: ../modules/packer/custom-image/README.md
[image-builder.yaml]: ./image-builder.yaml

#### Custom Network (deployment group 1)

A tool called [Packer](https://packer.io) builds custom VM images by creating
short-lived VMs, executing scripts on them, and saving the boot disk as an
image that can be used by future VMs. The short-lived VM typically operates in a
network that has outbound access to the internet for downloading software.

This deployment group creates a network using [Cloud Nat][cloudnat] and
[Identity-Aware Proxy (IAP)][iap] to allow outbound traffic and inbound SSH
connections without exposing the machine to the internet on a public IP address.

[cloudnat]: https://cloud.google.com/nat/docs/overview
[iap]: https://cloud.google.com/iap/docs/using-tcp-forwarding

#### Toolkit Runners (deployment group 1)

The Toolkit [startup-script](../modules/scripts/startup-script/README.md)
module supports boot-time configuration of VMs using "runners". Runners are
configured as a series of scripts uploaded to Cloud Storage. A simple, standard
[VM startup script][vmstartup] runs at boot-time, downloads the scripts from
Cloud Storage and executes them in sequence.

The script in this example performs the trivial task of creating a file as a
simple demonstration of functionality. You can use the startup-script module
to address more complex scenarios.

[vmstartup]: https://cloud.google.com/compute/docs/instances/startup-scripts/linux

#### Packer Template (deployment group 2)

The Packer module uses the startup-script module from the first deployment group
and executes the script to produce a custom image.

#### Slurm Cluster Based on Custom Image (deployment group 3)

Once the Slurm cluster has been deployed we can test that our Slurm compute
partition is using the custom image. Each compute node should contain the
`hello.txt` file added by the startup-script.

1. SSH into the login node `slurm-image-builder-001-login0`.
2. Run a job that prints the contents of the added file:

  ```bash
  $ srun -N 2 cat /home/hello.txt
  Hello World
  Hello World
  ```

#### Quota Requirements for image-builder.yaml

For this example the following is needed in the selected region:

* Compute Engine API: Images (global, not regional quota): 1 image per invocation of `packer build`
* Compute Engine API: Persistent Disk SSD (GB): **~50 GB**
* Compute Engine API: Persistent Disk Standard (GB): **~64 GB static + 32
  GB/node** up to 704 GB
* Compute Engine API: N2 CPUs: **4** (for short-lived Packer VM and Slurm login node)
* Compute Engine API: C2 CPUs: **4** for controller node and **60/node** active
  in `compute` partition up to 1,204
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for `compute` partition_

### [cloud-batch.yaml] ![core-badge]

This example demonstrates how to use the HPC Toolkit to set up a Google Cloud Batch job
that mounts a Filestore instance and runs startup scripts.

The blueprint creates a Filestore and uses the `startup-script` module to mount
and load _"data"_ onto the shared storage. The `batch-job-template` module creates
an instance template to be used for the Google Cloud Batch compute VMs and
renders a Google Cloud Batch job template. A login node VM is created with
instructions on how to SSH to the login node and submit the Google Cloud Batch
job.

[cloud-batch.yaml]: ../examples/cloud-batch.yaml

### [batch-mpi.yaml] ![core-badge]

This blueprint demonstrates how to use Spack to run a real MPI job on Batch.

The blueprint contains the following:

* A shared `filestore` filesystem.
* A `spack-install` module that builds a script to install Spack and the WRF
  application onto the shared `filestore`.
* A `startup-script` module which uses the above script and stages job data.
* A builder `vm-instance` which performs the Spack install and then shuts down.
* A `batch-job-template` that builds a Batch job to execute the WRF job.
* A `batch-login` VM that can be used to test and submit the Batch job.

**Usage instructions:**

1. Spack install

    After `terraform apply` completes, you must wait for Spack installation to
    finish before running the Batch job. You will observe that a VM named
    `spack-builder-0` has been created. This VM will automatically shut down
    once Spack installation has completed. When using a Spack cache this takes
    about 25 minutes. Without a Spack cache this will take 2 hours. To view
    build progress or debug you can inspect `/var/logs/messages` and
    `/var/log/spack.log` on the builder VM.

2. Access login node

    After the builder shuts down, you can ssh to the Batch login node named
    `batch-wrf-batch-login`. Instructions on how to ssh to the login node are
    printed to the terminal after a successful `terraform apply`. You can
    reprint these instructions by calling the following:

    ```sh
    terraform -chdir=batch-wrf/primary output instructions_batch-login
    ```

    Once on the login node you should be able to inspect the Batch job template
    found in the `/home/batch-jobs` directory. This Batch job will call a script
    found at `/share/wrfv3/submit_wrfv3.sh`. Note that the `/share` directory is
    shared between the login node and the Batch job.

3. Submit the Batch job

    Use the command provided in the terraform output instructions to submit your
    Batch job and check its status. The Batch job may take several minutes to
    start and once running should complete within 5 minutes.

4. Inspect results

    The Batch job will create a folder named `/share/jobs/<unique id>`. Once the
    job has finished this folder will contain the results of the job. You can
    inspect the `rsl.out.0000` file for a summary of the job.

[batch-mpi.yaml]: ../examples/batch-mpi.yaml

### [lustre.yaml] ![core-badge]

Creates a DDN EXAScaler lustre file-system that is mounted in two client instances.

The [DDN Exascaler Lustre](../community/modules/file-system/DDN-EXAScaler/README.md)
file system is designed for high IO performance. It has a default capacity of ~10TiB and is mounted at `/lustre`.

> **Warning**: The DDN Exascaler Lustre file system has a license cost as
> described in the pricing section of the
> [DDN EXAScaler Cloud Marketplace Solution](https://console.developers.google.com/marketplace/product/ddnstorage/).

After the creation of the file-system and the client instances, the lustre drivers will be automatically installed and the mount-point configured on the VMs. This may take a few minutes after the VMs are created and can be verified by running:

```sh
watch mount -t lustre
```

#### Quota Requirements for lustre.yaml

For this example the following is needed in the selected region:

* Compute Engine API: Persistent Disk SSD (GB): **~14TB: 3500GB MDT, 3500GB OST[0-2]**
* Compute Engine API: Persistent Disk Standard (GB): **~756GB: 20GB MDS, 276GB MGS, 3x20GB OSS, 2x200GB client-vms**
* Compute Engine API: N2 CPUs: **~116: 32 MDS, 32 MGS, 3x16 OSS, 2x2 client-vms**

[lustre.yaml]: ./lustre.yaml

### [slurm-gcp-v5-hpc-centos7.yaml] ![community-badge]

This example creates an HPC cluster similar to the one created by
[hpc-cluster-small.yaml], but uses modules built from version 5 of
[slurm-gcp].

The cluster will support 2 partitions named `debug` and `compute`.
The `debug` partition is the default partition and runs on smaller
`n2-standard-2` nodes. The `compute` partition is not default and requires
specifying in the `srun` command via the `--partition` flag. The `compute`
partition runs on compute optimized nodes of type `cs-standard-60`. The
`compute` partition may require additional quota before using.

#### Quota Requirements for slurm-gcp-v5-hpc-centos7.yaml

For this example the following is needed in the selected region:

* Cloud Filestore API: Basic HDD (Standard) capacity (GB): **1,024 GB**
* Compute Engine API: Persistent Disk SSD (GB): **~50 GB**
* Compute Engine API: Persistent Disk Standard (GB): **~50 GB static + 50
  GB/node** up to 1,250 GB
* Compute Engine API: N2 CPUs: **12**
* Compute Engine API: C2 CPUs: **4** for controller node and **60/node** active
  in `compute` partition up to 1,204
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for `compute` partition_

[slurm-gcp-v5-hpc-centos7.yaml]: ../community/examples/slurm-gcp-v5-hpc-centos7.yaml
[slurm-gcp]: https://github.com/SchedMD/slurm-gcp/tree/5.2.0

### [slurm-gcp-v5-ubuntu2004.yaml] ![community-badge]

Similar to the previous example, but using Ubuntu 20.04 instead of CentOS 7.
[Other operating systems] are supported by SchedMD for the the Slurm on GCP project and images are listed [here](https://github.com/SchedMD/slurm-gcp/blob/master/docs/images.md#published-image-family). Only the examples listed in this page been tested by the Cloud HPC Toolkit team.

This example creates an HPC cluster similar to the one created by
[hpc-cluster-small.yaml], but uses modules built from version 5 of
[slurm-gcp] and Ubuntu.

The cluster will support 2 partitions named `debug` and `compute`.
The `debug` partition is the default partition and runs on smaller
`n2-standard-2` nodes. The `compute` partition is not default and requires
specifying in the `srun` command via the `--partition` flag. The `compute`
partition runs on compute optimized nodes of type `cs-standard-60`. The
`compute` partition may require additional quota before using.

[Other operating systems]: https://github.com/SchedMD/slurm-gcp/blob/master/docs/images.md#supported-operating-systems
[slurm-gcp-v5-ubuntu2004.yaml]: ../community/examples/slurm-gcp-v5-ubuntu2004.yaml

#### Quota Requirements for slurm-gcp-v5-ubuntu2004.yaml

For this example the following is needed in the selected region:

* Cloud Filestore API: Basic HDD (Standard) capacity (GB): **1,024 GB**
* Compute Engine API: Persistent Disk SSD (GB): **~50 GB**
* Compute Engine API: Persistent Disk Standard (GB): **~50 GB static + 50
  GB/node** up to 1,250 GB
* Compute Engine API: N2 CPUs: **12**
* Compute Engine API: C2 CPUs: **4** for controller node and **60/node** active
  in `compute` partition up to 1,204
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for `compute` partition_

### [slurm-gcp-v5-high-io.yaml] ![community-badge]

This example uses [Slurm on GCP][slurm-gcp] version 5.x modules to replicate the
[hpc-cluster-high-io.yaml] core example. With version 5, additional features are
available and utilized in this example:

* node groups are used to allow multiple machine types in a single partition,
  differentiated by node names.
* Active cluster reconfiguration is on by default. When updating a partition or
  cluster configuration, the overwrite option (`-w`) can be used and upon
  re-applying the deployment, the changes will become active without having to
  destroy and recreate the cluster.

This blueprint will create a cluster with the following storage tiers:

* The homefs mounted at `/home` is a default "BASIC_HDD" tier filestore with
  1 TiB of capacity
* The projectsfs is mounted at `/projects` and is a high scale SSD filestore
  instance with 10TiB of capacity.
* The scratchfs is mounted at `/scratch` and is a
  [DDN Exascaler Lustre](../community/modules/file-system/DDN-EXAScaler/README.md)
  file system designed for high IO performance. The capacity is ~10TiB.

> **Warning**: The DDN Exascaler Lustre file system has a license cost as
> described in the pricing section of the
> [DDN EXAScaler Cloud Marketplace Solution](https://console.developers.google.com/marketplace/product/ddnstorage/).

The cluster will support 2 partitions:

* `lowcost`
  * Includes two node groups, `n2s2` of machine type `n2-standard-2` and `n2s4`
    of machine type `n2-standard-4`.
  * Default partition.
  * Designed to run with lower cost nodes and within a typical project's default
    quota.
* `compute`
  * Includes two node groups, `c2s60` of machine type `c2-standard-60` and
    `c2s30` of machine type `c2-standard-30`.
  * Can be used by setting the `--partition` option in `srun` to `compute`.
  * Designed for performance, but may require additional quota before using.

[slurm-gcp-v5-high-io.yaml]: ../community/examples/slurm-gcp-v5-high-io.yaml

#### Usage of Node Groups
This example defines partitions with more than one node group each. For more
information on node groups and why they are used, see the documentation in the
[schedmd-slurm-gcp-v5-node-group] module documentation. Some reference commands
are listed here for specifying not only the partition, but also the correct node
group when executing a Slurm command on a cluster generated by this blueprint.

Partition: compute; Node Group: c2s30; Machine Type: c2-standard-30

```bash
srun -N 4 -p compute -w highioslur-compute-c2s30-[0-3] hostname
```

Partition: compute; Node Group: c2s60; Machine Type: c2-standard-60

```bash
srun -N 4 -p compute --mincpus=30 hostname
```

Partition: lowcost; Node Group: n2s2; Machine Type: n2-standard-2

```bash
srun -N 4 -w highioslur-lowcost-n2s2-[0-3] hostname
```

Partition: lowcost; Node Group: n2s4; Machine Type: n2-standard-4

```bash
srun -N 4 --mincpus=2 hostname
```

[schedmd-slurm-gcp-v5-node-group]: ../community/modules/compute/schedmd-slurm-gcp-v5-node-group/README.md

#### Quota Requirements for slurm-gcp-v5-high-io.yaml

For this example the following is needed in the selected region:

* Cloud Filestore API: Basic HDD (Standard) capacity (GB) per region: **1,024 GB**
* Cloud Filestore API: High Scale SSD capacity (GB) per region: **10,240 GiB** - _min
  quota request is 61,440 GiB_
* Compute Engine API: Persistent Disk SSD (GB): **~14,050 GB**
* Compute Engine API: Persistent Disk Standard (GB): **~396 GB static + 20
  GB/node** up to 4596 GB
* Compute Engine API: N2 CPUs:
  * **4** for the login node
  * **2** per node for active nodes in the `n2s2` group, maximum 20.
  * **4** per node for active nodes in the `n2s4` group, maximum 40.
  * Maximum possible: **64**
* Compute Engine API: C2 CPUs:
  * **8** for controller node
  * **60** per node for active nodes in the `c2s60` group, maximum 12,000.
  * **30** per node for active nodes in the `c2s30` group, maximum 6,000.
  * Maximum possible: **18,008**
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for `compute` partition_

### [hpc-cluster-intel-select.yaml] ![community-badge]

This example provisions a Slurm cluster automating the [steps to comply to the
Intel Select Solutions for Simulation & Modeling Criteria][intelselect]. It is
more extensively discussed in a dedicated [README for Intel
examples][intel-examples-readme].

[hpc-cluster-intel-select.yaml]: ../community/examples/intel/hpc-cluster-intel-select.yaml
[intel-examples-readme]: ../community/examples/intel/README.md
[intelselect]: https://cloud.google.com/compute/docs/instances/create-intel-select-solution-hpc-clusters

### [daos-cluster.yaml] ![community-badge]

This example provisions a DAOS cluster with [managed instance groups][migs] for the servers and for clients. It is more extensively discussed in a dedicated [README for Intel
examples][intel-examples-readme].

[daos-cluster.yaml]: ../community/examples/intel/daos-cluster.yaml
[migs]: https://cloud.google.com/compute/docs/instance-groups

### [daos-slurm.yaml] ![community-badge]

This example provisions DAOS servers and a Slurm cluster. It is
more extensively discussed in a dedicated [README for Intel
examples][intel-examples-readme].

[daos-slurm.yaml]: ../community/examples/intel/daos-slurm.yaml

### [hpc-cluster-amd-slurmv5.yaml] ![community-badge]

This example provisions a Slurm cluster using AMD VM machine types. It
automates the initial setup of Spack, including a script that can be used to
install the AMD Optimizing C/C++ Compiler ([AOCC]) and compile OpenMPI with
AOCC. It is more extensively discussed in a dedicated [README for AMD
examples][amd-examples-readme].

[hpc-cluster-amd-slurmv5.yaml]: ../community/examples/AMD/hpc-cluster-amd-slurmv5.yaml
[AOCC]: https://developer.amd.com/amd-aocc/
[amd-examples-readme]: ../community/examples/AMD/README.md

### [quantum-circuit-simulator.yaml] ![community-badge]

This blueprint provisions a [N1 series VM with NVIDIA T4 GPU accelerator][t4]
and compiles [qsim], a [Google Quantum AI][gqai]-developed tool that simulates
quantum circuits using CPUs and GPUs. The installation of qsim, the [CUDA
Toolkit][cudatk], and the [cuQuantum SDK][cqsdk] is fully automated but takes a
significant time (approx. 20 minutes). Once complete, a qsim example can be run
by connecting to the VM by SSH and running

```shell
conda activate qsim
python /var/tmp/qsim-example.py
```

[gqai]: https://quantumai.google/
[quantum-circuit-simulator.yaml]: ../community/examples/quantum-circuit-simulator.yaml
[t4]: https://cloud.google.com/compute/docs/gpus#nvidia_t4_gpus
[qsim]: https://quantumai.google/qsim
[cqsdk]: https://developer.nvidia.com/cuquantum-sdk
[cudatk]: https://developer.nvidia.com/cuda-toolkit

### [spack-gromacs.yaml] ![community-badge] ![experimental-badge]

Spack is an HPC software package manager. This example creates a small Slurm
cluster with software installed using the
[spack-install module](../community/modules/scripts/spack-install/README.md) The
controller will install and configure spack, and install
[gromacs](https://www.gromacs.org/) using spack. Spack is installed in a shared
location (/sw) via filestore. This build leverages the
[startup-script module](../modules/scripts/startup-script/README.md) and can be
applied in any cluster by using the output of spack-install or
startup-script modules.

The installation will occur as part of the Slurm startup-script, a warning
message will be displayed upon SSHing to the login node indicating
that configuration is still active. To track the status of the overall
startup script, run the following command on the login node:

```shell
sudo tail -f /var/log/messages
```

Spack specific installation logs will be sent to the spack_log as configured in
your blueprint, by default /var/log/spack.log in the login node.

```shell
sudo tail -f /var/log/spack.log
```

Once the Slurm and Spack configuration is complete, spack will be available on
the login node. To use spack in the controller or compute nodes, the following
command must be run first:

```shell
source /sw/spack/share/spack/setup-env.sh
```

To load the gromacs module, use spack:

```shell
spack load gromacs
```

> **_NOTE:_** Installing spack compilers and libraries in this example can take
> hours to run on startup. To decrease this time in future deployments, consider
> including a spack build cache as described in the comments of the example.

[spack-gromacs.yaml]: ../community/examples/spack-gromacs.yaml

### [omnia-cluster.yaml] ![community-badge] ![experimental-badge]

Creates a simple [Dell Omnia][omnia-github] provisioned cluster with an
omnia-manager node that acts as the slurm manager and 2 omnia-compute nodes on
the pre-existing default network. Omnia will be automatically installed after
the nodes are provisioned. All nodes mount a filestore instance on `/home`.

> **_NOTE:_** The omnia-cluster.yaml example uses `vm-instance` modules to
> create the cluster. For these instances, Simultaneous Multithreading (SMT) is
> turned off by default, meaning that only the physical cores are visible. For
> the compute nodes, this means that 30 physical cores are visible on the
> `c2-standard-60` VMs. To activate all 60 virtual cores, include
> `threads_per_core=2` under settings for the compute vm-instance module.

[omnia-github]: https://github.com/dellhpc/omnia
[omnia-cluster.yaml]: ../community/examples/omnia-cluster.yaml

### [hpc-cluster-small-sharedvpc.yaml] ![community-badge] ![experimental-badge]

This blueprint demonstrates the use of the Slurm and Filestore modules in
the service project of an existing Shared VPC.  Before attempting to deploy the
blueprint, one must first complete [initial setup for provisioning Filestore in
a Shared VPC service project][fs-shared-vpc].

[hpc-cluster-small-sharedvpc.yaml]: ../community/examples/hpc-cluster-small-sharedvpc.yaml
[fs-shared-vpc]: https://cloud.google.com/filestore/docs/shared-vpc

### [hpc-cluster-localssd.yaml] ![community-badge] ![experimental-badge]

This blueprint demonstrates the use of Slurm and Filestore, with the definition
of a partition which deploys compute nodes that have local ssd drives deployed.
Before deploying this blueprint, one must first ensure to have an existing VPC
properly configured (allowing Internet access and allowing inter virtual
machine communications, for NFS and also for communications between the Slurm
nodes)

[hpc-cluster-localssd.yaml]: ../community/examples/hpc-cluster-localssd.yaml

### [htcondor-pool.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions an auto-scaling [HTCondor][htcondor] pool based upon
the [HPC VM Image][hpcvmimage].

Also see the [tutorial](../docs/tutorials/README.md#htcondor-tutorial), which
walks through the use of this blueprint.

[htcondor]: https://htcondor.org/
[htcondor-pool.yaml]: ../community/examples/htcondor-pool.yaml
[hpcvmimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm

### [gke.yaml] ![community-badge] ![experimental-badge]

This blueprint uses GKE to provision a Kubernetes cluster with a system node
pool (included in gke-cluster module) and an autoscaling compute node pool. It
creates a VPC configured to be used by a VPC native GKE cluster with subnet
secondary IP ranges defined.

The `gke-job-template` module is used to create a job file that can be submitted
to the cluster using `kubectl` and will run on the specified node pool.

[gke.yaml]: ../community/examples/gke.yaml

### [starccm-tutorial.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions a simple cluster for use with a Simcenter StarCCM+
tutorial.

> The main tutorial is described on the [HPC Toolkit website](https://cloud.google.com/hpc-toolkit/docs/simcenter-star-ccm/run-workload).

[starccm-tutorial.yaml]: ../community/examples/starccm-tutorial.yaml

### [fluent-tutorial.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions a simple cluster for use with an Ansys Fluent
tutorial.

> The main tutorial is described on the [HPC Toolkit website](https://cloud.google.com/hpc-toolkit/docs/tutorials/ansys-fluent).

[fluent-tutorial.yaml]: ../community/examples/fluent-tutorial.yaml

## Blueprint Schema

Similar documentation can be found on
[Google Cloud Docs](https://cloud.google.com/hpc-toolkit/docs/setup/hpc-blueprint).

A user defined blueprint should follow the following schema:

```yaml
# Required: Name your blueprint.
blueprint_name: my-blueprint-name

# Top-level variables, these will be pulled from if a required variable is not
# provided as part of a module. Any variables can be set here by the user,
# labels will be treated differently as they will be applied to all created
# GCP resources.
vars:
  # Required: This will also be the name of the created deployment directory.
  deployment_name: first_deployment
  project_id: GCP_PROJECT_ID

# https://cloud.google.com/compute/docs/regions-zones
  region: us-central1
  zone: us-central1-a

# https://cloud.google.com/resource-manager/docs/creating-managing-labels
  labels:
    global_label: label_value

# Many modules can be added from local and remote directories.
deployment_groups:
- group: groupName
  modules:

  # Local source, prefixed with ./ (/ and ../ also accepted)
  - id: <a unique id> # Required: Name of this module used to uniquely identify it.
    source: ./modules/role/module-name # Required: Points to the module directory.
    kind: < terraform | packer > # Optional: Type of module, currently choose from terraform or packer. If not specified, `kind` will default to `terraform`
    # Optional: All configured settings for the module. For terraform, each
    # variable listed in variables.tf can be set here, and are mandatory if no
    # default was provided and are not defined elsewhere (like the top-level vars)
    settings:
      setting1: value1
      setting2:
        - value2a
        - value2b
      setting3:
        key3a: value3a
        key3b: value3b

  # Embedded module (part of the toolkit), prefixed with modules/
  - source: modules/role/module-name

  # GitHub module over SSH, prefixed with git@github.com
  - source: git@github.com:org/repo.git//modules/role/module-name

  # GitHub module over HTTPS, prefixed with github.com
  - source: github.com/org/repo//modules/role/module-name
```

## Writing an HPC Blueprint

The blueprint file is composed of 3 primary parts, top-level parameters,
deployment variables and deployment groups. These are described in more detail
below.

### Blueprint Boilerplate

The following is a template that can be used to start writing a blueprint from
scratch.

```yaml
---
blueprint_name: # boilerplate-blueprint

vars:
  project_id: # my-project-id
  deployment_name: # boilerplate-001
  region: us-central1
  zone: us-central1-a

deployment_groups:
- group: primary
  modules:
  - id: # network1
    source: # modules/network/vpc
```

### Top Level Parameters

* **blueprint_name** (required): This name can be used to track resources and
  usage across multiple deployments that come from the same blueprint.
  `blueprint_name` is used as a value for the `ghpc_blueprint` label key, and
   must abide to label value naming constraints: `blueprint_name` must be at most
   63 characters long, and can only contain lowercase letters, numeric
   characters, underscores and dashes.

### Deployment Variables

```yaml
vars:
  region: "us-west-1"
  labels:
    "user-defined-deployment-label": "slurm-cluster"
  ...
```

Deployment variables are set under the vars field at the top level of the
blueprint file. These variables can be explicitly referenced in modules as
[Blueprint Variables](#blueprint-variables). Any module setting (inputs) not
explicitly provided and matching exactly a deployment variable name will
automatically be set to these values.

Deployment variables should be used with care. Module default settings with the
same name as a deployment variable and not explicitly set will be overwritten by
the deployment variable.

#### Deployment Variable "labels"

The “labels” deployment variable is a special case as it will be appended to
labels found in module settings, whereas normally an explicit module setting
would be left unchanged. This ensures that deployment-wide labels can be set
alongside module specific labels. Precedence is given to the module specific
labels if a collision occurs. Default module labels will still be overwritten by
deployment labels.

The HPC Toolkit uses special reserved labels for monitoring each deployment.
These are set automatically, but can be overridden in vars or module settings.
They include:

* ghpc_blueprint: The name of the blueprint the deployment was created from
* ghpc_deployment: The name of the specific deployment
* ghpc_role: See below

A module role is a default label applied to modules (`ghpc_role`), which
conveys what role that module plays within a larger HPC environment.

The modules provided with the HPC toolkit have been divided into roles
matching the names of folders in the [modules/](../modules/) and
[community/modules](../community/modules/) directories (compute,
file-system etc.).

When possible, custom modules should use these roles so that they match other
modules defined by the toolkit. If a custom module does not fit into these
roles, a new role can be defined.

A module's parent folder will define the module’s role if possible. Therefore,
regardless of where the module is located, the module directory should be
explicitly referenced at least 2 layers deep, where the top layer refers to the
“role” of that module.

If a module is not defined at least 2 layers deep and the `ghpc_role` label has
not been explicitly set in settings, ghpc_role will default to `undefined`.

Below we show some of the core modules and their roles (as parent folders).

```text
modules/
└── <<ROLE>
    └── <<MODULE_NAME>>

modules/
├── compute
│   └── vm-instance
├── file-system
│   ├── pre-existing-network-storage
│   └── filestore
├── monitoring
│   └── dashboard
├── network
│   ├── pre-existing-vpc
│   └── vpc
├── packer
│   └── custom-image
└── scripts
    └── startup-script
```

### Deployment Groups

Deployment groups allow distinct sets of modules to be defined and deployed as a
group. A deployment group can only contain modules of a single kind, for example
a deployment group may not mix packer and terraform modules.

For terraform modules, a top-level main.tf will be created for each deployment
group so different groups can be created or destroyed independently.

A deployment group is made of 2 fields, group and modules. They are described in
more detail below.

#### Group

Defines the name of the group. Each group must have a unique name. The name will
be used to create the subdirectory in the deployment directory.

#### Modules

Modules are the building blocks of an HPC environment. They can be composed in a
blueprint file to create complex deployments. Several modules are provided by
default in the [modules](../modules/README.md) folder.

To learn more about how to refer to a module in a blueprint file, please consult the
[modules README file.](../modules/README.md)

## Variables

Variables can be used to refer both to values defined elsewhere in the blueprint
and to the output and structure of other modules.

### Blueprint Variables

Variables in a blueprint file can refer to deployment variables or the outputs
of other modules. For deployment and module variables, the syntax is as follows:

```yaml
vars:
  zone: us-central1-a

deployment_groups:
  - group: primary
     modules:
       - id: resource1
         source: path/to/module/1
         ...
       - id: resource2
         source: path/to/module/2
         ...
         settings:
            key1: $(vars.zone)
            key2: $(resource1.name)
```

The variable is referred to by the source, either vars for deploment variables
or the module ID for module variables, followed by the name of the value being
referenced. The entire variable is then wrapped in “$()”.

Currently, references to variable attributes and string operations with
variables are not supported.

### Literal Variables

Literal variables are not interpreted by `ghpc` directly, but rather embedded in the
underlying module. Literal variables should only be used by those familiar
with the underlying module technology (Terraform or Packer); no validation
will be done before deployment to ensure that they are referencing
something that exists.

Literal variables are occasionally needed when referring to the data structure
of the underlying module. For example, to refer to the subnetwork self link from
a vpc module through terraform itself:

```yaml
subnetwork_self_link: ((module.network1.primary_subnetwork.self_link))
```

Here the network1 module is referenced, the terraform module name is the same as
the ID in the blueprint file. From the module we can refer to it's underlying
variables as deep as we need, in this case the self_link for it's
primary_subnetwork.

The entire text of the variable is wrapped in double parentheses indicating that
everything inside will be provided as is to the module.

Whenever possible, blueprint variables are preferred over literal variables.
`ghpc` will perform basic validation making sure all blueprint variables are
defined before creating a deployment, making debugging quicker and easier.

### Escape Variables

Under circumstances where the variable notation conflicts with the content of a setting or string, for instance when defining a startup-script runner that uses a subshell like in the example below, a non-quoted backslash (`\`) can be used as an escape character. It preserves the literal value of the next character that follows:

* `\$(not.bp_var)` evaluates to `$(not.bp_var)`.
* `\((not.literal_var))` evaluates to `((not.literal_var))`.

```yaml
deployment_groups:
  - group: primary
     modules:
       - id: resource1
         source: path/to/module/1
         settings:
            key1: \((not.literal_var))   ## Evaluates to "((not.literal_var))".
         ...
       - id: resource2
         source: path/to/module/2
         ...
         settings:
            key1: |
              #!/bin/bash
              echo \$(cat /tmp/file1)    ## Evaluates to "echo $(cat /tmp/file1)"
```
