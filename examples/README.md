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
  * [hpc-cluster-small.yaml](#hpc-cluster-smallyaml-)
  * [hpc-cluster-high-io.yaml](#hpc-cluster-high-ioyaml-)
  * [slurm-gcp-v5-hpc-centos7.yaml](#slurm-gcp-v5-hpc-centos7yaml-)
  * [slurm-gcp-v5-ubuntu2004.yaml](#slurm-gcp-v5-ubuntu2004yaml-)
  * [image-builder.yaml](#image-builderyaml-)
  * [hpc-cluster-intel-select.yaml](#hpc-cluster-intel-selectyaml-)
  * [daos-cluster.yaml](#daos-clusteryaml-)
  * [daos-slurm.yaml](#daos-slurmyaml-)
  * [cloud-batch.yaml](#cloud-batchyaml--)
  * [spack-gromacs.yaml](#spack-gromacsyaml--)
  * [omnia-cluster.yaml](#omnia-clusteryaml--)
  * [hpc-cluster-small-sharedvpc.yaml](#hpc-cluster-small-sharedvpcyaml--)
  * [htcondor-pool.yaml](#htcondor-poolyaml--)
* [Blueprint Schema](#blueprint-schema)
* [Writing an HPC Blueprint](#writing-an-hpc-blueprint)
  * [Top Level Parameters](#top-level-parameters)
  * [Deployment Variables](#deployment-variables)
  * [Deployment Groups](#deployment-groups)
* [Variables](#variables)
  * [Blueprint Variables](#blueprint-variables)
  * [Literal Variables](#literal-variables)

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
[slurm-gcp]: https://github.com/SchedMD/slurm-gcp/tree/v5.0.2

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

### [image-builder.yaml] ![core-badge]

This Blueprint uses the [Packer template module][pkr] to create custom VM images
by applying software and configurations to existing images.

This example performs the following:

1. Creates a network needed to build the image (see
   [Custom Network](#custom-network-deployment-group-1)).
2. Sets up a script that will be used to configure the image (see
   [Toolkit Runners](#toolkit-runners-deployment-group-1)).
3. Builds a new image by modifying the Slurm image (see
   [Packer Template](#packer-template-deployment-group-2)).
4. Deploys a Slurm cluster using the newly built image (see
   [Slurm Cluster Based on Custom Image](#slurm-cluster-based-on-custom-image-deployment-group-3)).

> **Note**: this example relies on the default behavior of the Toolkit to derive
> naming convention for networks and other modules from the `deployment_name`.

The commands needed to run through this example would look like:

```bash
# Create a deployment from the blueprint
./ghpc create examples/image-builder.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}"

# Deploy the network for packer (1) and generate the startup script (2)
terraform -chdir=image-builder-001/builder-env init
terraform -chdir=image-builder-001/builder-env validate
terraform -chdir=image-builder-001/builder-env apply

# Provide startup script to Packer
terraform -chdir=image-builder-001/builder-env output \
  -raw startup_script_scripts_for_image > \
  image-builder-001/packer/custom-image/startup_script.sh

# Build image (3)
cd image-builder-001/packer/custom-image
packer init .
packer validate -var startup_script_file=startup_script.sh .
packer build -var startup_script_file=startup_script.sh .

# Deploy Slurm cluster (4)
cd -
terraform -chdir=image-builder-001/cluster init
terraform -chdir=image-builder-001/cluster validate
terraform -chdir=image-builder-001/cluster apply

# When you are done you can clean up the resources in reverse order of creation
terraform -chdir=image-builder-001/cluster destroy --auto-approve
terraform -chdir=image-builder-001/builder-env destroy --auto-approve
```

Using a custom VM image can be more scalable than installing software using
boot-time startup scripts because:

* it avoids reliance on continued availability of package repositories
* VMs will join an HPC cluster and execute workloads more rapidly due to reduced
  boot-time configuration
* machines are guaranteed to boot with a static set of packages available when
  the custom image was created. No potential for some machines to be upgraded
  relative to other based upon their creation time!

[hpcimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm
[pkr]: ../modules/packer/custom-image/README.md
[image-builder.yaml]: ./image-builder.yaml

#### Custom Network (deployment group 1)

A tool called [Packer](https://packer.io) builds custom VM images by creating
short-lived VMs, executing scripts on them, and saving the boot disk as an
image that can be used by future VMs. The short-lived VM must operate in a
network that

* has outbound access to the internet for downloading software
* has SSH access from the machine running Packer so that local files/scripts
  can be copied to the VM

This deployment group creates such a network, while using [Cloud Nat][cloudnat]
and [Identity-Aware Proxy (IAP)][iap] to allow outbound traffic and inbound SSH
connections without exposing the machine to the internet on a public IP address.

[cloudnat]: https://cloud.google.com/nat/docs/overview
[iap]: https://cloud.google.com/iap/docs/using-tcp-forwarding

#### Toolkit Runners (deployment group 1)

The Toolkit [startup-script](../modules/scripts/startup-script/README.md)
module supports boot-time configuration of VMs using "runners". Runners are
configured as a series of scripts uploaded to Cloud Storage. A simple, standard
[VM startup script][vmstartup] runs at boot-time, downloads the scripts from
Cloud Storage and executes them in sequence.

The standard bash startup script is exported as a string by the startup-script
module.

The script in this example is performing the trivial task of creating a file in
the image's home directory just to demonstrate the capability. You can expand
the startup-script module to install more complex dependencies.

[vmstartup]: https://cloud.google.com/compute/docs/instances/startup-scripts/linux

#### Packer Template (deployment group 2)

The Packer template in this deployment group accepts [several methods for
executing custom scripts][pkr]. To pass the exported startup string to it, you
must collect it from the Terraform module and provide it to the Packer template.
After running `terraform -chdir=image-builder-001/builder-env apply` as
instructed by `ghpc`, execute the following:

```shell
terraform -chdir=image-builder-001/builder-env \
  output -raw startup_script_install_ansible > \
  image-builder-001/packer/custom-image/startup_script.sh
cd image-builder-001/packer/custom-image
packer init .
packer validate -var startup_script_file=startup_script.sh .
packer build -var startup_script_file=startup_script.sh .
```

#### Quota Requirements for image-builder.yaml

For this example the following is needed in the selected region:

* Compute Engine API: Images (global, not regional quota): 1 image per invocation of `packer build`
* Compute Engine API: Persistent Disk SSD (GB): **~50 GB**
* Compute Engine API: Persistent Disk Standard (GB): **~20 GB static + 20
  GB/node** up to 500 GB
* Compute Engine API: N2 CPUs: **4** (for short-lived Packer VM and Slurm login node)
* Compute Engine API: C2 CPUs: **4** for controller node and **60/node** active
  in `compute` partition up to 1,204
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for `compute` partition_

#### Slurm Cluster Based on Custom Image (deployment group 3)

Once the Slurm cluster has been deployed we can test that our Slurm compute
partition is now using the image we built. It should contain the `hello.txt`
file that was added during image build:

1. SSH into the login node `slurm-image-builder-001-login0`.
2. Run a job that prints the contents of the added file:

  ```bash
  $ srun -N 2 cat /home/hello.txt
  Hello World
  Hello World
  ```

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

### [cloud-batch.yaml] ![community-badge] ![experimental-badge]

This example demonstrates how to use the HPC Toolkit to set up a Google Cloud Batch job
that mounts a Filestore instance and runs startup scripts.

The blueprint creates a Filestore and uses the `startup-script` module to mount
and load _"data"_ onto the shared storage. The `cloud-batch-job` module creates
an instance template to be used for the Google Cloud Batch compute VMs and
renders a Google Cloud Batch job template. A login node VM is created with
instructions on how to SSH to the login node and submit the Google Cloud Batch
job.

[cloud-batch.yaml]: ../community/examples/cloud-batch.yaml

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
omnia-manager node and 2 omnia-compute nodes on the pre-existing default
network. Omnia will be automatically installed after the nodes are provisioned.
All nodes mount a filestore instance on `/home`.

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

### [htcondor-pool.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions an auto-scaling [HTCondor][htcondor] pool based upon
the [HPC VM Image][hpcvmimage].

[htcondor]: https://htcondor.org/
[htcondor-pool.yaml]: ../community/examples/htcondor-pool.yaml
[hpcvmimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm

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
    kind: < terraform | packer > # Required: Type of module, currently choose from terraform or packer.
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
