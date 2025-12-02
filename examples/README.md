# Example Blueprints

## AI Hypercomputer

Additional blueprints optimized for AI workloads on modern GPUs is available at [Google Cloud AI Hypercomputer][aihc]. Documentation is available for [GKE][aihc-gke] and for [Slurm][aihc-slurm].

[aihc]: https://cloud.google.com/ai-hypercomputer/docs
[aihc-gke]: https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute
[aihc-slurm]: https://cloud.google.com/ai-hypercomputer/docs/create/create-slurm-cluster

<!-- TOC generated with some manual tweaking of the following command output:
md_toc github examples/README.md | sed -e "s/\s-\s/ * /"
-->
<!-- TOC -->

* [Instructions](#instructions)
  * [(Optional) Setting up a remote terraform state](#optional-setting-up-a-remote-terraform-state)
* [Blueprint Descriptions](#blueprint-descriptions)
  * [hpc-slurm.yaml](#hpc-slurmyaml-) ![core-badge]
  * [hpc-enterprise-slurm.yaml](#hpc-enterprise-slurmyaml-) ![core-badge]
  * [hpc-slurm-static.yaml](#hpc-slurm-staticyaml-) ![core-badge]
  * [hpc-slurm6-tpu.yaml](#hpc-slurm6-tpuyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm6-tpu-maxtext.yaml](#hpc-slurm6-tpu-maxtextyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm6-apptainer.yaml](#hpc-slurm6-apptaineryaml--) ![community-badge] ![experimental-badge]
  * [ml-slurm.yaml](#ml-slurmyaml-) ![core-badge]
  * [h4d-vm.yaml](#h4d-vmyaml--) ![core-badge] ![experimental-badge]
  * [image-builder.yaml](#image-builderyaml-) ![core-badge]
  * [serverless-batch.yaml](#serverless-batchyaml-) ![core-badge]
  * [serverless-batch-mpi.yaml](#serverless-batch-mpiyaml-) ![core-badge]
  * [pfs-lustre.yaml](#pfs-lustreyaml-) ![core-badge] ![deprecated-badge]
  * [pfs-managed-lustre-vms.yaml](#pfs-managed-lustre-vmsyaml-) ![core-badge]
  * [gke-managed-lustre.yaml](#gke-managed-lustreyaml-) ![core-badge]  
  * [ps-slurm.yaml](#ps-slurmyaml--) ![core-badge] ![experimental-badge]
  * [cae-slurm.yaml](#cae-slurmyaml-) ![core-badge]
  * [hpc-build-slurm-image.yaml](#hpc-build-slurm-imageyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-ubuntu2004.yaml](#hpc-slurm-ubuntu2004yaml--) ![community-badge]
  * [hpc-amd-slurm.yaml](#hpc-amd-slurmyaml-) ![community-badge]
  * [hpc-slurm-sharedvpc.yaml](#hpc-slurm-sharedvpcyaml--) ![community-badge] ![experimental-badge]
  * [client-google-cloud-storage.yaml](#client-google-cloud-storageyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-gromacs.yaml](#hpc-slurm-gromacsyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-local-ssd.yaml](#hpc-slurm-local-ssdyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-h4d.yaml](#hpc-slurm-h4dyaml--) ![core-badge]
  * [hpc-slinky.yaml](#hpc-slinkyyaml--) ![community-badge] ![experimental-badge]
  * [hcls-blueprint.yaml](#hcls-blueprintyaml-) ![core-badge]
  * [af3-slurm.yaml](#af3-slurmyaml--) ![core-badge] ![experimental-badge]
  * [hpc-gke.yaml](#hpc-gkeyaml-) ![core-badge]
  * [ml-gke](#ml-gkeyaml-) ![core-badge]
  * [storage-gke](#storage-gkeyaml-) ![core-badge]
  * [gke-managed-hyperdisk.yaml](#gke-managed-hyperdiskyaml--) ![core-badge] ![experimental-badge]
  * [gke-a3-ultragpu.yaml](#gke-a3-ultragpuyaml-) ![core-badge]
  * [gke-a3-megagpu](#gke-a3-megagpuyaml-) ![core-badge]
  * [gke-a3-highgpu](#gke-a3-highgpuyaml-) ![core-badge]
  * [gke-a3-highgpu-inference-gateway.yaml](#gke-a3-highgpu-inference-gatewayyaml-) ![core-badge]
  * [gke-consumption-options](#gke-consumption-options-) ![core-badge]
  * [htc-slurm.yaml](#htc-slurmyaml-) ![community-badge]
  * [htc-htcondor.yaml](#htc-htcondoryaml--) ![community-badge] ![experimental-badge]
  * [fsi-montecarlo-on-batch.yaml](#fsi-montecarlo-on-batchyaml-) ![community-badge] ![experimental-badge]
  * [tutorial-starccm-slurm.yaml](#tutorial-starccm-slurmyaml--) ![community-badge] ![experimental-badge]
  * [tutorial-starccm.yaml](#tutorial-starccmyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-ramble-gromacs.yaml](#hpc-slurm-ramble-gromacsyaml--) ![community-badge] ![experimental-badge]
  * [flux-cluster](#flux-clusteryaml--) ![community-badge] ![experimental-badge]
  * [tutorial-fluent.yaml](#tutorial-fluentyaml--) ![community-badge] ![experimental-badge]
  * [gke-tpu-v6](#gke-tpu-v6--) ![community-badge] ![experimental-badge]
  * [xpk-n2-filestore](#xpk-n2-filestore--) ![community-badge] ![experimental-badge]
  * [gke-h4d](#gke-h4d-) ![core-badge]
  * [gke-g4](#gke-g4-) ![core-badge]
* [Blueprint Schema](#blueprint-schema)
* [Writing an HPC Blueprint](#writing-an-hpc-blueprint)
  * [Blueprint Boilerplate](#blueprint-boilerplate)
  * [Top Level Parameters](#top-level-parameters)
  * [Deployment Variables](#deployment-variables)
  * [Deployment Groups](#deployment-groups)
* [Variables and expressions](#variables-and-expressions)

## Instructions

Ensure `project_id`, `zone`, and `region` deployment variables are set correctly
under `vars` before using an example blueprint.

> **_NOTE:_** Deployment variables defined under `vars` are automatically passed
> to modules if the modules have an input that matches the variable name.

### (Optional) Setting up a remote terraform state

There are two ways to specify [terraform backends] in Cluster Toolkit: a default setting that propagates all groups and custom per-group configuration:

* `terraform_backend_defaults` at top-level of YAML blueprint
* `terraform_backend` within a deployment group definition

Examples of each are shown below. If both settings are used, then the custom per-group value is used without modification.

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

All Terraform-supported backends are supported by the Toolkit. Specify the backend using `type` and its [configuration block] using `configuration`.

For the [gcs] backend, you must minimally supply the `bucket` configuration setting. The `prefix` setting is generated automatically as "blueprint_name/deployment_name/group_name" for each deployment group. This ensures uniqueness.

If you wish to specify a custom prefix, use a unique value for each group following this example:

```yaml
deployment_groups:
- group: example_group
  terraform_backend:
    type: gcs
    configuration:
      bucket: your-bucket
      prefix: your/object/prefix
```

You can set the configuration using the CLI in the `create` and `expand`
subcommands as well:

```shell
./gcluster create examples/hpc-slurm.yaml \
  --vars "project_id=${GOOGLE_CLOUD_PROJECT}" \
  --backend-config "bucket=${GCS_BUCKET}"
```

> **_NOTE:_** The `--backend-config` argument supports comma-separated list of
> name=value variables to set Terraform Backend configuration in blueprints.
> This feature only supports variables of string type. If you set configuration
> in both the blueprint and CLI, the tool uses values at CLI. "gcs" is set as
> type by default.

[terraform backends]: https://developer.hashicorp.com/terraform/language/settings/backends/configuration
[configuration block]: https://developer.hashicorp.com/terraform/language/settings/backends/configuration#using-a-backend-block
[gcs]: https://developer.hashicorp.com/terraform/language/settings/backends/gcs

## Blueprint Descriptions

[core-badge]: https://img.shields.io/badge/-core-blue?style=plastic
[community-badge]: https://img.shields.io/badge/-community-%23b8def4?style=plastic
[stable-badge]: https://img.shields.io/badge/-stable-lightgrey?style=plastic
[experimental-badge]: https://img.shields.io/badge/-experimental-%23febfa2?style=plastic
[deprecated-badge]: https://img.shields.io/badge/-deprecated-%23fea2a2?style=plastic

The example blueprints listed below labeled with the core badge
(![core-badge]) are located in this folder and are developed and tested by the
Cluster Toolkit team directly.

The community blueprints are contributed by the community (including the Cluster
Toolkit team, partners, etc.) and are labeled with the community badge
(![community-badge]). The community blueprints are located in the
[community folder](../community/examples/).

Blueprints that are still in development and less stable are also labeled with
the experimental badge (![experimental-badge]).

### [hpc-slurm.yaml] ![core-badge]

Creates a basic auto-scaling Slurm cluster with mostly default settings. The
blueprint also creates a new VPC network, and a filestore instance mounted to
`/home`.

There are 3 partitions in this example: `debug` `compute`, and `h3`. The `debug`
partition uses `n2-standard-2` VMs, which should work out of the box without
needing to request additional quota. The purpose of the `debug` partition is to
make sure that first time users are not immediately blocked by quota
limitations.

[hpc-slurm.yaml]: ./hpc-slurm.yaml

#### Compute Partition

There is a `compute` partition that achieves higher performance. Any
performance analysis should be done on the `compute` partition. By default it
uses `c2-standard-60` VMs with placement groups enabled. You may need to request
additional quota for `C2 CPUs` in the region you are deploying in. You can
select the compute partition using the `-p compute` argument when running `srun`.

#### H3 Partition

There is an `h3` partition that uses compute-optimized `h3-standard-88` machine type.
You can read more about the H3 machine series [here](https://cloud.google.com/compute/docs/compute-optimized-machines#h3_series).

#### Quota Requirements for hpc-slurm.yaml

For this example the following is needed in the selected region:

* Cloud Filestore API: Basic HDD (Standard) capacity (GB): **1,024 GB**
* Compute Engine API: Persistent Disk SSD (GB): **~50 GB**
* Compute Engine API: Persistent Disk Standard (GB): **~50 GB static + 50
  GB/node** up to 1,250 GB
* Compute Engine API: N2 CPUs: **2** for the login node and **2/node** active
  in the `debug` partition up to 12
* Compute Engine API: C2 CPUs: **4** for the controller node and **60/node**
  active in the `compute` partition up to 1,204
* Compute Engine API: H3 CPUs: **88/node** active in the `h3` partition up to
  1760
  * The H3 CPU quota can be increased on the Cloud Console by navigating to
  `IAM & Admin`->`Quotas` or searching `All Quotas` and entering `vm_family:H3`
  into the filter bar.  From there, the quotas for each region may be selected
  and edited.
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for the `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for the `compute` partition_

### [hpc-enterprise-slurm.yaml] ![core-badge]

This advanced blueprint creates a cluster with Slurm with several performance
tunings enabled, along with tiered file systems for higher performance. Some of
these features come with additional cost and required additional quotas.

The Slurm system deployed here connects to the default VPC of the project and
creates a  login node and the following seven partitions:

* `n2` with general-purpose [`n2-standard-2` nodes][n2]. Placement policies and
exclusive usage are disabled, which means the nodes can be used for multiple jobs.
Nodes will remain idle for 5 minutes before Slurm deletes them. This partition can
be used for debugging and workloads that do not require high performance.
* `c2` with compute-optimized [`c2-standard-60` nodes][c2] based on Intel 3.9 GHz
Cascade Lake processors.
* `c2d` with compute optimized [`c2d-standard-112` nodes][c2d] base on the third
generation AMD EPYC Milan.
* `c3` with compute-optimized [`c3-highcpu-176` nodes][c3] based on Intel Sapphire
Rapids processors. When configured with Tier_1 networking, C3 nodes feature 200 Gbps
low-latency networking.
* `h3` with compute-optimized [`h3-standard-88` nodes][h3]  based on Intel Sapphire
Rapids processors. H3 VMs can use the entire host network bandwidth and come with a default network bandwidth rate of up to 200 Gbps.
* `a208` with [`a2-ultragpu-8g` nodes][a2] with 8 of the NVIDIA A100 GPU accelerators
with 80GB of GPU memory each.
* `a216` with [`a2-megagpu-16g` nodes][a2] with 16 of the NVIDIA A100 GPU accelerators
with 40GB of GPU memory each.

For all partitions other than `n2`, [compact placement] policies are enabled by default
and nodes are created and destroyed on a per-job basis. Furthermore, these partitions
are configured with:

* Faster networking: Google Virtual NIC ([GVNIC]) is used for the GPU partitions and
[Tier_1] is selected when available. Selecting Tier_1 automatically enables GVNIC.
* SSD PDs disks for compute nodes. See the [Storage options] page for more details.

[n2]: https://cloud.google.com/compute/docs/general-purpose-machines#n2_series
[c2]: https://cloud.google.com/compute/docs/compute-optimized-machines#c2_machine_types
[c2d]: https://cloud.google.com/compute/docs/compute-optimized-machines#c2d_machine_types
[c3]: https://cloud.google.com/blog/products/compute/introducing-c3-machines-with-googles-custom-intel-ipu
[h3]: https://cloud.google.com/compute/docs/compute-optimized-machines#h3_series
[a2]: https://cloud.google.com/compute/docs/gpus#a100-gpus
[g2]: https://cloud.google.com/compute/docs/gpus#l4-gpus
[compact placement]: https://cloud.google.com/compute/docs/instances/define-instance-placement
[GVNIC]: https://cloud.google.com/compute/docs/networking/using-gvnic
[Tier_1]: https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration
[Storage options]: https://cloud.google.com/compute/docs/disks

File systems:

* The homefs mounted at `/home` uses the "BASIC_SSD" tier filestore with
  2.5 TiB of capacity
* The projectsfs is mounted at `/projects` and is a high scale SSD filestore
  instance with 10TiB of capacity.
* The scratchfs is mounted at `/scratch` and is a
  [DDN Exascaler Lustre](../community/modules/file-system/DDN-EXAScaler/README.md)
  file system designed for high IO performance. The capacity is ~10TiB.

> [!WARNING]
> This module is deprecated and will be removed on July 1, 2025. The
> recommended replacement is the
> [GCP Managed Lustre module](../../../../modules/file-system/managed-lustre/README.md)

#### Quota Requirements for hpc-enterprise-slurm.yaml

For this example the following is needed in the selected region:

* Cloud Filestore API: Basic SSD capacity (GB) per region: **2,560 GB**
* Cloud Filestore API: High Scale SSD capacity (GB) per region: **10,240 GiB** -
  _min quota request is 61,440 GiB_
* Compute Engine API: Persistent Disk SSD (GB): **~14,050 GB** static +
  **100 GB/node** up to 23,250 GB
* Compute Engine API: Persistent Disk Standard (GB): **~396 GB** static +
  **50 GB/node** up to 596 GB
* Compute Engine API: N2 CPUs: **116** for login and lustre and **2/node** active
 in `n2` partition up to 124.
* Compute Engine API: C2 CPUs: **4** for controller node and **60/node** active
  in `c2` partition up to 1,204
* Compute Engine API: C2D CPUs: **112/node** active in `c2d` partition up to 2,240
* Compute Engine API: C3 CPUs: **176/node** active in `c3` partition up to 3,520
* Compute Engine API: H3 CPUs: **88/node** active in `h3` partition up to 1,408
* Compute Engine API: A2 CPUs: **96/node** active in `a208` and `a216` partitions
up to 3,072
* Compute Engine API: NVIDIA A100 80GB GPUs: **8/node** active in `a208` partition
 up to 128
* Compute Engine API: NVIDIA A100 GPUs: **8/node** active in `a216` partition up
to 256
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _not needed for `n2` partition_

[hpc-enterprise-slurm.yaml]: ./hpc-enterprise-slurm.yaml

### [hpc-slurm-static.yaml] ![core-badge]

This example demonstrates how to create a partition with static compute nodes.
See [Best practices for static compute nodes] for instructions on setting up a
reservation and compact placement policy.

Before deploying this example the following fields must be populated in the bluerpint:

```yaml
  project_id: ## Set GCP Project ID Here ##
  static_reservation_name:  ## Set your reservation name here ##
  static_reservation_machine_type: ## Machine must match reservation above ##
  static_node_count: ## Must be <= number of reserved machines ##
```

For more resources on static compute nodes see the following cloud docs pages:

* [About [Slurm] node types](https://cloud.google.com/cluster-toolkit/docs/slurm/node-types)
* [Best practices for static compute nodes]
* [Reconfigure a running cluster](http://cloud/cluster-toolkit/docs/slurm/reconfigure-cluster)
* [Manage static compute nodes](http://cloud/cluster-toolkit/docs/slurm/manage-static-nodes)

For a similar, more advanced, example which demonstrates static node
functionality with GPUs, see the
[ML Slurm A3 example](./machine-learning/README.md).

[Best practices for static compute nodes]: http://cloud/cluster-toolkit/docs/slurm/static-nodes-best-practices
[hpc-slurm-static.yaml]: ./hpc-slurm-static.yaml

### [hpc-slurm6-tpu.yaml] ![community-badge] ![experimental-badge]

Creates an auto-scaling Slurm cluster with TPU nodes.

[hpc-slurm6-tpu.yaml]: ../community/examples/hpc-slurm6-tpu.yaml

### [hpc-slurm6-tpu-maxtext.yaml] ![community-badge] ![experimental-badge]

Creates an auto-scaling Slurm cluster with TPU nodes.

For tutorial on how to run [maxtext] workload on TPU partition using Slurm,
Follow [hpc-slurm-tpu-maxtext].

[maxtext]: https://github.com/google/maxtext
[hpc-slurm6-tpu-maxtext.yaml]: ../community/examples/hpc-slurm6-tpu-maxtext.yaml
[hpc-slurm-tpu-maxtext]: ../docs/hpc-slurm6-tpu-maxtext.md

### [hpc-slurm6-apptainer.yaml] ![community-badge] ![experimental-badge]

This blueprint creates a custom [Apptainer](https:https://apptainer.org) enabled image and builds an auto-scaling Slurm cluster using that image. You can deploy containerized workloads on that cluster as described [here](https://github.com/GoogleCloudPlatform/scientific-computing-examples/tree/main/apptainer).

[hpc-slurm6-apptainer.yaml]: ../community/examples/hpc-slurm6-apptainer.yaml

### [h4d-vm.yaml] ![core-badge] ![experimental-badge]

This blueprint deploys a cluster containing a pair of `h4d-highmem-192-lssd` VMs with RDMA networking enabled along with a filestore instance mounted to `/home`.

[h4d-vm.yaml]: ../examples/h4d-vm.yaml

### [ml-slurm.yaml] ![core-badge]

This blueprint provisions an HPC cluster running the Slurm scheduler with the
machine learning frameworks PyTorch and TensorFlow pre-installed on every
VM. The cluster has 2 partitions:

* [A2 family VMs][a2] with the NVIDIA A100 GPU accelerator
* [G2 family VMs][g2] with the NVIDIA L4 GPU accelerator

[a2]: https://cloud.google.com/compute/docs/gpus#a100-gpus
[g2]: https://cloud.google.com/compute/docs/gpus#l4-gpus

To provision the cluster, please run:

```text
./gcluster create examples/ml-slurm.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}"
./gcluster deploy ml-example-v6
```

After accessing the login node, you can activate the conda environment for each
library with:

```shell
source /etc/profile.d/conda.sh
# to activate PyTorch
conda activate pytorch
# to activate TensorFlow
conda activate tf
```

An example benchmarking job for PyTorch can be run under Slurm:

```shell
cp /var/tmp/torch_test.* .
sbatch -N 1 --gpus-per-node=1 torch_test.sh
```

When you are done, clean up the resources in reverse order of creation:

```text
./gcluster destroy ml-example-v6
```

Finally, browse to the [Cloud Console][console-images] to delete your custom
image. It will be named beginning with `ml-slurm` followed by a date and
timestamp for uniqueness.

[ml-slurm.yaml]: ../examples/ml-slurm.yaml

### [image-builder.yaml] ![core-badge]

This blueprint uses the [Packer template module][pkr] to create a custom VM
image and uses it to provision an HPC cluster using the Slurm scheduler. By
using a custom image, the cluster is able to begin running jobs sooner and more
reliably because there is no need to install applications as VMs boot. This
example takes the following steps:

1. Creates a network with outbound internet access in which to build the image (see
[Custom Network](#custom-network-deployment-group-1)).
2. Creates a script that will be used to customize the image (see
[Toolkit Runners](#toolkit-runners-deployment-group-1)).
3. Builds a custom Slurm image by executing the script on a standard Slurm image
(see [Packer Template](#packer-template-deployment-group-2)).
4. Deploys a Slurm cluster using the custom image (see
[Slurm Cluster Based on Custom Image](#slurm-cluster-based-on-custom-image-deployment-group-3)).

#### Building and using the custom image

Create the deployment folder from the blueprint:

```text
./gcluster create examples/image-builder.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}"
./gcluster deploy image-builder-v6-001"
```

Follow the on-screen prompts to approve the creation of each deployment group.
For example, the network is created in the first deployment group, the VM image
is created in the second group, and the third group uses the image to create an
HPC cluster using the Slurm scheduler.

When you are done, clean up the resources in reverse order of creation:

```text
terraform -chdir=image-builder-v6-001/cluster destroy --auto-approve
terraform -chdir=image-builder-v6-001/primary destroy --auto-approve
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

1. SSH into the login node `imagebuild-login-login-001`.
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

### [serverless-batch.yaml] ![core-badge]

This example demonstrates how to use the Cluster Toolkit to set up a Google Cloud Batch job
that mounts a Filestore instance and runs startup scripts.

The blueprint creates a Filestore and uses the `startup-script` module to mount
and load _"data"_ onto the shared storage. The `batch-job-template` module creates
an instance template to be used for the Google Cloud Batch compute VMs and
renders a Google Cloud Batch job template. A login node VM is created with
instructions on how to SSH to the login node and submit the Google Cloud Batch
job.

[serverless-batch.yaml]: ../examples/serverless-batch.yaml

### [serverless-batch-mpi.yaml] ![core-badge]

This blueprint demonstrates how to use Spack to run a real MPI job on Batch.

The blueprint contains the following:

* A shared `filestore` filesystem.
* A `spack-setup` module that generates a script to install Spack
* A `spack-execute` module that builds the WRF application onto the shared
  `filestore`.
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

[serverless-batch-mpi.yaml]: ../examples/serverless-batch-mpi.yaml

### [pfs-lustre.yaml] ![core-badge] ![deprecated-badge]

_This blueprint has been deprecated and will be removed on August 1, 2025._

Creates a DDN EXAScaler lustre file-system that is mounted in two client instances.

The [DDN Exascaler Lustre](../community/modules/file-system/DDN-EXAScaler/README.md)
file system is designed for high IO performance. It has a default capacity of
~10TiB and is mounted at `/lustre`.

> **Warning**: The DDN Exascaler Lustre file system has a license cost as
> described in the pricing section of the
> [DDN EXAScaler Cloud Marketplace Solution](https://console.developers.google.com/marketplace/product/ddnstorage/).

After the creation of the file-system and the client instances, the startup
scripts on the client instances will automatically install the lustre drivers,
configure the mount-point, and mount the file system to the specified
directory. This may take a few minutes after the VMs are created and can be
verified by running:

```sh
watch df
```

Eventually you should see a line similar to:

```sh
<IP>:<remote_mount>  lustre   100G   15G  85G  15% <local_mount>
```

with remote_mount, and local_mount reflecting the settings of the module and
IP being set to the lustre instance's IP.

#### Quota Requirements for pfs-lustre.yaml

For this example the following is needed in the selected region:

* Compute Engine API: Persistent Disk SSD (GB): **~14TB: 3500GB MDT, 3500GB OST[0-2]**
* Compute Engine API: Persistent Disk Standard (GB): **~756GB: 20GB MDS, 276GB MGS, 3x20GB OSS, 2x200GB client-vms**
* Compute Engine API: N2 CPUs: **~116: 32 MDS, 32 MGS, 3x16 OSS, 2x2 client-vms**

[pfs-lustre.yaml]: ./pfs-lustre.yaml

### [pfs-managed-lustre-vms.yaml] ![core-badge]

Creates a Managed Lustre file-system that is mounted in one client instance.

The [GCP Managed Lustre](../modules/file-system/managed-lustre/README.md)
file system is designed for high IO performance. It has a minimum capacity of ~18TiB and is mounted at `/lustre`.

After the creation of the file-system and the client instances, the startup
scripts on the client instances will automatically install the lustre drivers,
configure the mount-point, and mount the file system to the specified
directory. This may take a few minutes after the VMs are created and can be
verified by running:

```sh
watch df
```

Eventually you should see a line similar to:

```sh
<IP>:<remote_mount>  lustre   100G   15G  85G  15% <local_mount>
```

with remote_mount, and local_mount reflecting the settings of the module and
IP being set to the lustre instance's IP.

#### Quota Requirements for pfs-managed-lustre.yaml

For this example, the following is needed in the selected region:

* Compute Engine API: Persistent Disk SSD (GB): **~800GB: 800GB MDT**
* Compute Engine API: Persistent Disk Standard (GB): **~328GB: 128 MGT, 200GB client-vm**
* Compute Engine API: Hyperdisk Balanced (GB): **~27432GB: 18432 GB OST Pool, 8*1125GB OST**
* Compute Engine API: N2 CPUs: **~34: 32 MGS, 2 client-vm**
* Compute Engine API: C3 CPUs: **~396: 44 MDS, 2*176 OSS**

[pfs-managed-lustre-vms.yaml]: ./pfs-managed-lustre-vms.yaml

### [gke-managed-lustre.yaml] ![core-badge]

This Cluster Toolkit blueprint deploys a Google Kubernetes Engine (GKE) cluster integrated with Google Cloud Managed Lustre,
providing a high-performance file system for demanding workloads.

#### Features

* **VPC Network:** Sets up a new VPC, subnet, and secondary ranges for GKE pods and services.
* **Private Services Access:** Configures Private Services Access, required for Managed Lustre.
* **Firewall Rules:** Creates firewall rules to allow traffic between GKE nodes and the Managed Lustre instance (port 988).
* **Managed Lustre Instance:** Provisions a Google Cloud Managed Lustre file system instance.
* **Service Accounts:** Creates dedicated service accounts for GKE node pools and workloads with necessary IAM roles.
* **GKE Cluster:** Deploys a GKE cluster with the Managed Lustre CSI driver enabled (`enable_managed_lustre_csi: true`).
* **Persistent Volume:** Creates a Kubernetes PersistentVolume (PV) and PersistentVolumeClaim (PVC) to make the Managed Lustre instance accessible to pods.
* **GKE Node Pool:** Sets up a node pool where application pods can run and mount the Lustre file system.

#### Requirements

1. **Cluster Toolkit:** Ensure you have the Cluster Toolkit (`gcluster`) binary built and ready to use.
2. **GCP Project:** A Google Cloud Project with necessary permissions to create VPCs, GKE clusters, Managed Lustre instances, and related resources.
3. **Quotas:** Sufficient quotas for GCE, GKE, and Managed Lustre resources in the selected region. Note that Managed Lustre capacity and performance tiers have specific quota requirements. See [Managed Lustre Performance Tiers](https://cloud.google.com/managed-lustre/docs/create-instance#performance-tiers) and [Quotas](https://cloud.google.com/managed-lustre/docs/quotas).
4. **GKE Version:** The blueprint is configured for GKE version `1.33.x` or later, as required by the Managed Lustre CSI driver.
5. **Location:** Managed Lustre is only available in specific regions and zones. Verify and adjust based on [Managed Lustre Locations](https://cloud.google.com/managed-lustre/docs/locations).

#### Steps to deploy the blueprint

1. Install Cluster Toolkit
    1. Install [dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
    1. Set up [Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).

1. Switch to the Cluster Toolkit directory

   ```sh
   cd cluster-toolkit
   ```

1. Get the IP address for your host machine

   ```sh
   curl ifconfig.me
   ```

1. Update the vars block of the blueprint file
    1. `project_id`: ID of the project where you are deploying the cluster.
    1. `deployment_name`: Name of the deployment.
    1. `region / zone`: Ensure these support Managed Lustre.
    1. `authorized_cidr`: update the IP address in <your-ip-address>/32.
    1. `size_gib`: Capacity of the Managed Lustre instance in GiB.
    1. `per_unit_storage_throughput`: Throughput in MB/s per TiB. The combination of size and throughput must match a valid performance tier.

1. Build the Cluster Toolkit binary

   ```sh
   make
   ```

1. Provision the GKE cluster

   ```sh
   ./gcluster deploy examples/gke-managed-lustre.yaml
   ```

   This process can take several minutes as it provisions the VPC, GKE cluster, Managed Lustre instance, and configures the CSI driver.

#### Accessing and Using Managed Lustre

1. Configure kubectl: After successful deployment, configure kubectl to connect to your new GKE cluster:

   ```sh
   gcloud container clusters get-credentials $(vars.deployment_name) \
   --region $(vars.region) \
   --project $(vars.project_id)
   ```

   Replace `$(vars.deployment_name)`, `$(vars.region)`, and `$(vars.project_id)` with the actual values from your blueprint.

1. Verify PVC: Check that the PersistentVolumeClaim has been created and is Bound:

   ```sh
   kubectl get pvc
   ```

   You should see a PVC named $(vars.lustre_instance_id)-pvc with STATUS: Bound

1. Example Pod: Create a file named lustre-client-pod.yaml to deploy a test pod that mounts the Lustre volume

   ```sh
   apiVersion: v1
   kind: Pod
   metadata:
     name: lustre-client-pod
   spec:
     containers:
     - name: app
       image: busybox
       command: ["/bin/sh", "-c", "sleep 36000"] # Keep container running
       volumeMounts:
       - mountPath: "/mnt/lustre"
         name: lustre-volume
     volumes:
     - name: lustre-volume
       persistentVolumeClaim:
         claimName: $(vars.lustre_instance_id)-pvc # Matches the PVC name  
   ```

   Note: This is just an example job using busybox image.

1. Deploy the Pod:

   ```sh
   kubectl apply -f lustre-pod.yaml
   ```

1. Verify Mount: Once the pod is running, exec into it to check the mount:

   ```sh
   kubectl exec -it lustre-client-pod -- /bin/sh
   # Inside the pod:
   df -h /mnt/lustre
   mount | grep lustre
   ```

#### Clean Up
To destroy all resources created by this blueprint, run:

   ```sh
   ./gcluster destroy CLUSTER-NAME
   ```

   Replace `CLUSTER-NAME` with the `deployment_name` used in blueprint vars block.

[gke-managed-lustre.yaml]: ../examples/gke-managed-lustre.yaml

### [cae-slurm.yaml] ![core-badge]

The Computer Aided Engineering (CAE) blueprint captures a reference architecture
where the right cloud components are assembled to optimally cater to the
requirements of computationally-intensive CAE workloads. Specifically, it is
architected around Google Cloud’s VM families that provide a high memory bandwidth
and a balanced memory/flop ratio, which is particularly useful for per-core licensed
CAE software. The solution caters also to large CAE use cases, requiring multiple nodes
that are tightly-coupled via MPI. Special high-memory shapes support even very
memory-demanding workloads with up to 16GB/core. For file IO, different Google managed
high performance NFS storage services are available. For very IO demanding workloads,
third party parallel file systems can be integrated. The scheduling of the workloads
is done by a workload manager.

The CAE blueprint is intended to be a starting point for more tailored explorations
or installations of specific CAE codes, as provided by ISVs separately.

A detailed documentation is provided in this [README](cae/README.md).

#### Quota Requirements for cae-slurm.yaml

For this example the following is needed in the selected region:

* Cloud Filestore API: Basic SSD capacity (GB) per region: **5,120 GB**
* Cloud Filestore API: High Scale SSD capacity (GB) per region: **10,240 GB**
* Compute Engine API: H3 CPUs: **88/node** active in `balance` partition up to 880
* Compute Engine API: C3-highmem CPUs: **176/node** active in `highmem` partition up to 1,760
* Compute Engine API: N1 CPUs: **8/node** active in `desktop` node.
* Compute Engine API: T4 GPUs: **1/node** active in `desktop` node.
* Compute Engine API: N2 CPUs: **8** for login and **16** for controller

[cae-slurm.yaml]: ../examples/cae/cae-slurm.yaml

### [hpc-build-slurm-image.yaml] ![community-badge] ![experimental-badge]

This blueprint demonstrates how to use Cluster Toolkit to build a Slurm image on top
of an existing image, `hpc-rocky-linux-8` in the case of this example.

The blueprint contains 3 groups:

1. The first group creates a network and generates the scripts that will install
   Slurm. This uses the Ansible Playbook contained in the
   [Slurm on GCP](https://github.com/GoogleCloudPlatform/slurm-gcp) repo.
2. The second group executes the build using Packer to run the scripts from the
   first group. This can take ~30 min and will generate a custom Slurm image in
   your project.
3. The third group deploys a demo cluster that uses the newly built image. For a
   real world use case the demo cluster can be swapped out for a more powerful
   slurm cluster from other examples.

[hpc-build-slurm-image.yaml]: ../community/examples/hpc-build-slurm-image.yaml

### [hpc-slurm-ubuntu2004.yaml] ![community-badge]

Similar to the [hpc-slurm.yaml] example, but using Ubuntu 20.04 instead of CentOS 7.
[Other operating systems] are supported by SchedMD for the the Slurm on GCP project and images are listed [here](https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/docs/images.md#published-image-family). Only the examples listed in this page been tested by the Cluster Toolkit team.

The cluster will support 2 partitions named `debug` and `compute`.
The `debug` partition is the default partition and runs on smaller
`n2-standard-2` nodes. The `compute` partition is not default and requires
specifying in the `srun` command via the `--partition` flag. The `compute`
partition runs on compute optimized nodes of type `cs-standard-60`. The
`compute` partition may require additional quota before using.

[Other operating systems]: https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/docs/images.md#supported-operating-systems
[hpc-slurm-ubuntu2004.yaml]: ../community/examples/hpc-slurm-ubuntu2004.yaml

#### Quota Requirements for hpc-slurm-ubuntu2004.yaml

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

### [hpc-amd-slurm.yaml] ![community-badge]

This example provisions a Slurm cluster using AMD VM machine types. It
automates the initial setup of Spack, including a script that can be used to
install the AMD Optimizing C/C++ Compiler ([AOCC]) and compile OpenMPI with
AOCC. It is more extensively discussed in a dedicated [README for AMD
examples][amd-examples-readme].

[hpc-amd-slurm.yaml]: ../community/examples/AMD/hpc-amd-slurm.yaml
[AOCC]: https://developer.amd.com/amd-aocc/
[amd-examples-readme]: ../community/examples/AMD/README.md

### [client-google-cloud-storage.yaml] ![community-badge] ![experimental-badge]

[client-google-cloud-storage.yaml]: ../community/examples/client-google-cloud-storage.yaml

This example demonstrates several different ways to use Google Cloud Storage
(GCS) buckets in the Cluster Toolkit. There are two buckets referenced in the
example:

1. A GCS bucket that is created by the Cluster Toolkit (`id: new-bucket`).
1. A GCS bucket that is created externally from the Cluster Toolkit but referenced
   by the blueprint (`id: existing-bucket`).

The created VM (`id: workstation`) references these GCS buckets with the `use`
field. On VM startup gcsfuse will be installed, if not already on the image, and
both buckets will be mounted under the directory specified by the `local_mount`
option.

The `wait-for-startup` module (`id: wait`) makes sure that terraform does not
exit before the buckets have been mounted.

To use the blueprint you must supply the project id and the name of an existing
bucket:

```shell
./gcluster create community/examples/client-google-cloud-storage.yaml \
  --vars project_id=<project_id> \
  --vars existing_bucket_name=<name_of_existing_bucket>
```

> **Note**: The service account used by the VM must have access to the buckets
> (`roles/storage.objectAdmin`). In this example the service account will
> default to the default compute service account.
>
> **Warning**: In this example the bucket is mounted by root during startup. Due
> to the way permissions are handled by gcsfuse this means that read or
> read/write permissions must be granted indiscriminantly for all users which
> could be a security concern depending on usage. To avoid this, you can
> manually mount as the user using the bucket
> ([Read more](https://github.com/GoogleCloudPlatform/gcsfuse/blob/master/docs/mounting.md#access-permissions)).

### [hpc-slurm-gromacs.yaml] ![community-badge] ![experimental-badge]

Spack is an HPC software package manager. This example creates a small Slurm
cluster with software installed using the
[spack-setup](../community/modules/scripts/spack-setup/README.md) and
[spack-execute](../community/modules/scripts/spack-execute/README.md) modules.
The controller will install and configure spack, and install
[gromacs](https://www.gromacs.org/) using spack. Spack is installed in a shared
location (/sw) via filestore. This build leverages the
[startup-script module](../modules/scripts/startup-script/README.md) and can be
applied in any cluster by using the output of spack-setup or startup-script
modules.

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

[hpc-slurm-gromacs.yaml]: ../community/examples/hpc-slurm-gromacs.yaml

### [hpc-slurm-ramble-gromacs.yaml] ![community-badge] ![experimental-badge]

Ramble is an experimentation framework which can drive the installation of
software with Spack and create, execute, and analyze experiments using the
installed software.

This example blueprint will deploy a Slurm cluster, install Spack and Ramble on
it, and create a Ramble workspace (named gromacs). This workspace can be setup using:

> **_NOTE:_** Since in this example installation of ramble is owned by
> `spack-ramble` user, you may consider running `sudo -i -u spack-ramble` first.

```shell
ramble workspace activate
ramble workspace setup
```

After setup is complete, the experiments can be executed using:

```shell
ramble workspace activate # If not active
ramble on
```

And after the experiments are complete, they can be analyzed using:

```shell
ramble workspace activate # If not active
ramble workspace analyze
```

The experiments defined by the workspace configuration are a 1, 2, 4, 8, and 16
node scaling study of the Lignocellulose benchmark for Gromacs.

[hpc-slurm-ramble-gromacs.yaml]: ../community/examples/hpc-slurm-ramble-gromacs.yaml

### [hpc-slurm-local-ssd.yaml] ![community-badge] ![experimental-badge]

This blueprint demonstrates the use of Slurm and Filestore, with compute nodes
that have local ssd drives deployed.

[hpc-slurm-local-ssd.yaml]: ../community/examples/hpc-slurm-local-ssd.yaml

### [hpc-slurm-h4d.yaml] ![core-badge]

Creates a basic auto-scaling Slurm cluster with mostly default settings. The
blueprint also creates two new VPC networks, one configured for RDMA networking and the other for non-RDMA networking, along with two filestore instances mounted to `/home` and `/apps`. There is an `h4d` partition that uses compute-optimized `h4d-highmem-192-lssd` machine type.

[hpc-slurm-h4d.yaml]: ../examples/hpc-slurm-h4d/hpc-slurm-h4d.yaml

### [hpc-slinky.yaml] ![community-badge] ![experimental-badge]

The SchedMD Slinky Project deploys Slurm on Kubernetes. Slinky is particularly useful for:
1. Those with a prefer a Slurm workload management paradigm, but a cloud-native operational experience
2. Those who want the flexibility of running HPC jobs with either Kubernetes-based scheduling or Slurm-based scheduling, all on the same platform

This blueprint creates a simple Slinky installation on top of Google Kubernetes Engine, with the following notable deviations from the Slinky quickstart setup:
1. Two nodesets are implemented, following the pattern of an HPC nodeset and a debug nodeset.
2. A login node is implemented.
3. A lightweight, GCP-native metrics/monitoring system is adopted, rather than the Slinky-documented cluster-local Kube Prometheus Stack.
4. Node affinities for system components, the login node, and compute nodesets are more explicitly defined, to improve stability, control, and HPC hardware utilization.

While H3 compute-optimized VMs are used for the HPC nodeset, the machine type can easily be switched (including to GPU-accelerated instances).

In order to create a static Slurm nodeset, which only requires one configuration to scale in/out (the nodeset's `replicas` setting), this example blueprint uses:
* Autoscaling GKE node pools (via `initial_node_count`)
* Non-autoscaling Slurm nodesets (via `replicas`), which sit 1:1 on top of the GKE nodes
If both of these settings were static, two changes would be required for nodeset scale outs - one at the Slurm level (nodeset replicas) and one at the infrastructure level (node pool node count) - so instead the node pool autoscales to "follow" the nodeset specification.

Scale in/out nodesets with a single `kubectl` command:

```bash
kubectl scale nodeset/slurm-compute-debug --replicas=5 -n slurm
```

Nodeset autoscaling is only possible with [KEDA installation and configuration work](https://github.com/SlinkyProject/slurm-operator/blob/main/docs/autoscaling.md), and this is not included in the example.

[hpc-slinky.yaml]: ../community/examples/hpc-slinky/hpc-slinky.yaml

### [hcls-blueprint.yaml]: ![core-badge]

This blueprint demonstrates an advanced architecture that can be used to run
GROMACS with GPUs and CPUs on Google Cloud. For full documentation, refer
[document].

[document]: ../docs/videos/healthcare-and-life-sciences/README.md
[hcls-blueprint.yaml]:  ../example/hcls-blueprint.yaml

### [af3-slurm.yaml]: ![core-badge] ![experimental-badge]

This blueprint lets you create a high-throughput execution environment for Google Deepmind's
[AlphaFold 3](https://blog.google/technology/ai/google-deepmind-isomorphic-alphafold-3-ai-model)
in your own GCP project. It uses the unmodified [AlphaFold 3 package](https://github.com/google-deepmind/alphafold3),
and provides a best-practices mapping of it to Google Cloud, leveraging Google Cloud's HPC technology.

We provide two simple examples that serve as basic templates for different ways of interacting with the
AlphaFold 3 solution:

* A Simple Job Launcher bash script that takes an AlphaFold 3 json file input (for the Datapipeline
step or the Inference step) and submits it for processing to the AlphaFold 3 autoscaling Slurm cluster.
* A Simple Service Launcher that has a central Python script that runs a loop monitoring directories on a
provided GCS bucket for input files and which can be started as a system daemon on the
controller-node, not requiring any user interaction with the AlphaFold 3 environment.

Before using this solution, please review the [AlphaFold 3 Model Parameter Terms of Use](https://github.com/google-deepmind/alphafold3/blob/main/WEIGHTS_TERMS_OF_USE.md).
Please check that you/your organization are eligible for obtaining the weights and that your use falls within the allowed terms and complies
with the [Prohibited Use Policy](https://github.com/google-deepmind/alphafold3/blob/main/WEIGHTS_PROHIBITED_USE_POLICY.md).

See the [AF3 Solution README] for more details.

[AF3 Solution README]: ../examples/science/af3-slurm/README.md
[af3-slurm.yaml]: ../examples/science/af3-slurm/af3-slurm.yaml

### [hpc-gke.yaml] ![core-badge]

This blueprint uses GKE to provision a Kubernetes cluster with a system node
pool (included in gke-cluster module) and an autoscaling compute node pool. It
creates a VPC configured to be used by a VPC native GKE cluster with subnet
secondary IP ranges defined.

The `gke-job-template` module is used to create a job file that can be submitted
to the cluster using `kubectl` and will run on the specified node pool.

#### Steps to deploy the blueprint

1. Install Cluster Toolkit
    1. Install [dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
    1. Set up [Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).
1. Switch to the Cluster Toolkit directory

   ```sh
   cd cluster-toolkit
   ```

1. Get the IP address for your host machine

   ```sh
   curl ifconfig.me
   ```

1. Update the vars block of the blueprint file
    1. `project_id`: ID of the project where you are deploying the cluster.
    1. `deployment_name`: Name of the deployment.
    1. `authorized_cidr`: update the IP address in <your-ip-address>/32.
1. Build the Cluster Toolkit binary

   ```sh
   make
   ```

1. Provision the GKE cluster

   ```sh
   ./gcluster deploy examples/hpc-gke.yaml
   ```

1. Run the job

    1. Connect to your cluster

       ```sh
       gcloud container clusters get-credentials CLUSTER_NAME --location=COMPUTE_REGION --project=PROJECT_ID
       ```

       * Update the `CLUSTER_NAME` to the `deployment_name`
       * Update the `COMPUTE_REGION` to the `region` used in blueprint vars
       * Update the `PROJECT_ID` to the `project_id` used in blueprint vars

    1. The output of the `./gcluster deploy` on CLI includes a `kubectl create` command to create the job.

       ```sh
       kubectl create -f <job-yaml-path> 
       ```

       This command creates a job that uses busybox image and prints `Hello World`. This result can be viewed by looking at the pod logs.

    1. List pods

       ```sh
       kubectl get pods
       ```

    1. Get the pod logs

       ```sh
       kubectl logs <pod-name>
       ```

#### Clean Up
To destroy all resources associated with creating the GKE cluster, from Cloud Shell run the following command:

```sh
./gcluster destroy CLUSTER-NAME
```

Replace `CLUSTER-NAME` with the `deployment_name` used in blueprint vars block.

[hpc-gke.yaml]: ../examples/hpc-gke.yaml

### [ml-gke.yaml] ![core-badge]

This blueprint demonstrates how to set up a GPU GKE cluster using the Cluster
Toolkit. It includes:

> **Warning**: `authorized_cidr` variable must be entered for this example to
> work. See note below.

* Creation of a regional GKE cluster.
* Creation of an autoscaling GKE node pool with `g2` machines.
  Note: This blueprint has also been tested with `a2` machines,
  but as capacity is hard to find the example uses `g2` machines which have better obtainability.
  If using with `a2` machines it is recommended to first obtain an automatic reservation.

  Example settings for a2 look like:

  ```yaml
  source: modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      disk_type: pd-balanced
      machine_type: a2-highgpu-2g
  ```

  Users only need to provide machine type for standard ["a2", "a3" and "g2"] machine families,
  while the other settings like `type`, `count` , `gpu_driver_installation_config` will default to
  machine family specific values. More on this [gke-node-pool](../community/modules/compute/gke-node-pool/README.md#gpus-examples)

```yaml
machine_type: n1-standard-1
guest_accelerator:
- type: nvidia-tesla-t4
  count: 1
```

Custom g2 pool with custom `guest_accelerator` config

```yaml
machine_type: g2-custom-16-55296
disk_type: pd-balanced
guest_accelerator:
- type: nvidia-l4
  count: 1
  gpu_sharing_config:
    max_shared_clients_per_gpu: 2
    gpu_sharing_strategy: "TIME_SHARING"
  gpu_driver_installation_config:
    gpu_driver_version: "LATEST"
```

* Configuration of the cluster using default drivers provided by GKE.
* Creation of a job template yaml file that can be used to submit jobs to the
  GPU node pool.

> **Note**: The Kubernetes API server will only allow requests from authorized
> networks. **You must use the
> `authorized_cidr` variable to supply an authorized network which contains the
> IP address of the machine deploying the blueprint, for example
> `--vars authorized_cidr=<your-ip-address>/32`.** This will allow Terraform to
> create the necessary DaemonSet on the cluster. You can use a service like
> [whatismyip.com](https://whatismyip.com) to determine your IP address.

Once you have deployed the blueprint, follow output instructions to _fetch
credentials for the created cluster_ and _submit a job calling `nvidia_smi`_.

[ml-gke.yaml]: ../examples/ml-gke.yaml
[`kubernetes-operations`]: ../community/modules/scripts/kubernetes-operations/README.md

### [storage-gke.yaml] ![core-badge]

This blueprint shows how to use different storage options with GKE in the toolkit.

The blueprint contains the following:

* A K8s Job that uses a Filestore and a GCS bucket as shared file systems between pods.
* A K8s Job that demonstrates different ephemeral storage options:
  * memory backed emptyDir
  * local SSD backed emptyDir
  * SSD persistent disk backed ephemeral volume
  * balanced persistent disk backed ephemeral volume

Note that when type `local-ssd` is used, the specified node pool must have
`local_ssd_count_ephemeral_storage` specified.

When using either `pd-ssd` or `pd-balanced` ephemeral storage, a persistent disk
will be created when the job is submitted. The disk will be automatically
cleaned up when the job is deleted.

> [!Note]
> The Kubernetes API server will only allow requests from authorized networks.
> The `gke-persistent-volume` module needs access to the Kubernetes API server
> to create a Persistent Volume and a Persistent Volume Claim. **You must use
> the `authorized_cidr` variable to supply an authorized network which contains
> the IP address of the machine deploying the blueprint, for example
> `--vars authorized_cidr=<your-ip-address>/32`.** You can use a service like
> [whatismyip.com](https://whatismyip.com) to determine your IP address.

[storage-gke.yaml]: ../examples/storage-gke.yaml

### [gke-managed-hyperdisk.yaml] ![core-badge] ![experimental-badge]

This blueprint shows how to use managed hyperdisk storage options with GKE in the toolkit.

The blueprint contains the following:

* A K8s Job that uses a managed hyperdisk storage volume option.
* A K8s Job that demonstrates ML training workload with managed hyperdisk storage disk operation.
  * The sample training workload manifest will be generated under the gke-managed-hyperdisk/primary folder, as tensorflow-GUID.yaml
  * You can deploy this sample training workload using "kubectl apply -f tensorflow-GUID.yaml" to start the training

> **Warning**: In this example blueprint, when storage type `Hyperdisk-balanced`, `Hyperdisk-extreme` or `Hyperdisk-throughput` is specified in `gke-storage` module.
> The lifecycle of the hyperdisk is managed by the blueprint.
> On glcuster destroy operation, the hyperdisk storage created will also be destroyed.
>
> [!Note]
> The Kubernetes API server will only allow requests from authorized networks.
> The `gke-cluster` module needs access to the Kubernetes API server
> to create a Persistent Volume and a Persistent Volume Claim. **You must use
> the `authorized_cidr` variable to supply an authorized network which contains
> the IP address of the machine deploying the blueprint, for example
> `--vars authorized_cidr=<your-ip-address>/32`.** You can use a service like
> [whatismyip.com](https://whatismyip.com) to determine your IP address.

#### Steps to deploy the blueprint

1. Install Cluster Toolkit
    1. Install [dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
    1. Set up [Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).
1. Switch to the Cluster Toolkit directory

   ```sh
   cd cluster-toolkit
   ```

1. Get the IP address for your host machine

   ```sh
   curl ifconfig.me
   ```

1. Update the vars block of the blueprint file
    1. `project_id`: ID of the project where you are deploying the cluster.
    1. `deployment_name`: Name of the deployment.
    1. `authorized_cidr`: update the IP address in <your-ip-address>/32.
1. Build the Cluster Toolkit binary

   ```sh
   make
   ```

1. Provision the GKE cluster

   ```sh
   ./gcluster deploy examples/gke-managed-hyperdisk.yaml
   ```

#### Clean Up
To destroy all resources associated with creating the GKE cluster, from Cloud Shell run the following command:

```sh
./gcluster destroy CLUSTER-NAME
```

Replace `CLUSTER-NAME` with the `deployment_name` used in blueprint vars block.

[gke-managed-hyperdisk.yaml]: ../examples/gke-managed-hyperdisk.yaml

### [gke-a3-ultragpu.yaml] ![core-badge]

Refer to [AI Hypercomputer Documentation](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#create-cluster) for instructions.

[gke-a3-ultragpu.yaml]: ../examples/gke-a3-ultragpu/gke-a3-ultragpu.yaml

### [gke-a3-megagpu.yaml] ![core-badge]

This blueprint shows how to provision a GKE cluster with A3 Mega machines in the toolkit. [Deploy an A3 Mega GKE cluster for ML training](https://cloud.google.com/cluster-toolkit/docs/deploy/deploy-a3-mega-gke-cluster) has the steps documented.

After provisioning the cluster and the nodepool, the below components will be installed
to enable GPUDirect for the A3 Mega machines.

* NCCL plugin for GPUDirect [TCPXO](https://github.com/GoogleCloudPlatform/container-engine-accelerators/tree/master/gpudirect-tcpxo)
* [NRI](https://github.com/GoogleCloudPlatform/container-engine-accelerators/tree/master/nri_device_injector) device injector plugin
* Provide support for injecting GPUDirect required components(annotations, volumes, rxdm sidecar etc.) into the user workload in the form of Kubernetes Job.
  * Provide sample workload to showcase how it will be updated with the required components injected, and how it can be deployed.
  * Allow user to use the provided script to update their own workload and deploy.

> [!Note]
> The Kubernetes API server will only allow requests from authorized networks.
> The `gke-cluster` module needs access to the Kubernetes API server
> to apply a manifest. **You must use
> the `authorized_cidr` variable to supply an authorized network which contains
> the IP address of the machine deploying the blueprint, for example
> `--vars authorized_cidr=<your-ip-address>/32`.** You can use a service like
> [whatismyip.com](https://whatismyip.com) to determine your IP address.

#### Troubleshooting

##### Externally Managed Environment Error

If you see an error saying: `local-exec provisioner error` or `This environment is externally managed`, please use a virtual environment. This error is caused due to a conflict between pip3 and the operating system's package manager (like apt on Debian/Ubuntu-based systems).

```shell
  ## One time step of creating the venv
  VENV_DIR=~/venvp3
  python3 -m venv $VENV_DIR
  ## Enter your venv.
  source $VENV_DIR/bin/activate
```

[gke-a3-megagpu.yaml]: ../examples/gke-a3-megagpu

### [gke-a3-highgpu.yaml] ![core-badge]

This blueprint shows how to provision a GKE cluster with A3 High machines in the toolkit.

After provisioning the cluster and the nodepool, the below components will be installed
to enable GPUDirect for the A3 High machines.

* NCCL plugin for GPUDirect [TCPX](https://github.com/GoogleCloudPlatform/container-engine-accelerators/tree/master/gpudirect-tcpx)
* [NRI](https://github.com/GoogleCloudPlatform/container-engine-accelerators/tree/master/nri_device_injector) device injector plugin
* Provide support for injecting GPUDirect required components(annotations, volumes, rxdm sidecar etc.) into the user workload in the form of Kubernetes Job via a script.

> [!Note]
> The Kubernetes API server will only allow requests from authorized networks.
> The `gke-cluster` module needs access to the Kubernetes API server
> to apply a manifest. **You must use
> the `authorized_cidr` variable to supply an authorized network which contains
> the IP address of the machine deploying the blueprint, for example
> `--vars authorized_cidr=<your-ip-address>/32`.** You can use a service like
> [whatismyip.com](https://whatismyip.com) to determine your IP address.

#### Troubleshooting

##### Externally Managed Environment Error

If you see an error saying: `local-exec provisioner error` or `This environment is externally managed`, please use a virtual environment. This error is caused due to a conflict between pip3 and the operating system's package manager (like apt on Debian/Ubuntu-based systems).

```shell
  ## One time step of creating the venv
  VENV_DIR=~/venvp3
  python3 -m venv $VENV_DIR
  ## Enter your venv.
  source $VENV_DIR/bin/activate
```

[gke-a3-highgpu.yaml]: ../examples/gke-a3-highgpu.yaml

### [gke-a3-highgpu-inference-gateway.yaml] ![core-badge]

This blueprint provisions a GKE cluster with A3 High machines, pre-configured to support the GKE Inference Gateway. It automates the setup of necessary networking components, such as a proxy-only subnet, and installs the required Custom Resource Definitions (CRDs) on the cluster.

After successfully deploying this blueprint, you can proceed with deploying a sample workload with vLLM inferencing by following the official guide at [Serve a model with GKE Inference Gateway](https://cloud.google.com/kubernetes-engine/docs/tutorials/serve-with-gke-inference-gateway).

This blueprint takes care of the initial infrastructure setup (e.g., network creation and CRD installation). You will need to follow the guide to install specific instances of `InferencePool`, `HTTPRoute`, and the `Model Server` deployment itself.

[gke-a3-highgpu-inference-gateway.yaml]: ../examples/gke-a3-highgpu-inference-gateway.yaml

### [gke-consumption-options] ![core-badge]

This folder holds multiple GKE blueprint examples that display different consumption options on GKE.
* [DWS Calendar](../examples/gke-consumption-options/dws-calendar)
* [DWS Flex Start](../examples/gke-consumption-options/dws-flex-start)
* [DWS Flex Start with Queued Provisioning](../examples/gke-consumption-options/dws-flex-start-queued-provisioning)

[gke-consumption-options]: ../examples/gke-consumption-options

### [htc-htcondor.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions an auto-scaling [HTCondor][htcondor] pool based upon
the [HPC VM Image][hpcvmimage].

Also see the [tutorial](../docs/tutorials/README.md#htcondor-tutorial), which
walks through the use of this blueprint.

[htcondor]: https://htcondor.org/
[htc-htcondor.yaml]: ../community/examples/htc-htcondor.yaml
[hpcvmimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm

### [htc-slurm.yaml] ![community-badge]

This blueprint provisions a cluster using the Slurm scheduler in a configuration
tuned for the execution of many short-duration, loosely-coupled (non-MPI) jobs.

For more information see:

* [Slurm on Google Cloud High Throughput documentation](https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/docs/htc.md)
* [General Slurm High Throughput documentation](https://slurm.schedmd.com/high_throughput.html)

[htc-slurm.yaml]: ../community/examples/htc-slurm.yaml

### [fsi-montecarlo-on-batch.yaml](../community/examples/fsi-montecarlo-on-batch.yaml) ![community-badge] ![experimental-badge]

## Monte Carlo Simulations for Value at Risk

This blueprint will take you through a tutorial on an FSI Value at Risk calculation using Cloud tools:

* Batch
* Pub/Sub
  * BigQuery pubsub subscription
* BigQuery
* Vertex AI Notebooks

See the [full tutorial here](../docs/tutorials/fsi-montecarlo-on-batch/README.md).

### [tutorial-starccm-slurm.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions an HPC cluster running Slurm for use with a Simcenter StarCCM+
tutorial.

> The main tutorial is described on the [Cluster Toolkit website](https://cloud.google.com/cluster-toolkit/docs/simcenter-starccm-slurm/run-workload).

[tutorial-starccm-slurm.yaml]: ../community/examples/tutorial-starccm-slurm.yaml

### [tutorial-starccm.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions a simple cluster for use with a Simcenter StarCCM+
tutorial.

> The main tutorial is described on the [Cluster Toolkit website](https://cloud.google.com/cluster-toolkit/docs/simcenter-star-ccm/run-workload).

[tutorial-starccm.yaml]: ../community/examples/tutorial-starccm.yaml

### [tutorial-fluent.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions a simple cluster for use with an Ansys Fluent
tutorial.

> The main tutorial is described on the [Cluster Toolkit website](https://cloud.google.com/cluster-toolkit/docs/tutorials/ansys-fluent).

[tutorial-fluent.yaml]: ../community/examples/tutorial-fluent.yaml

### [flux-cluster.yaml] ![community-badge] ![experimental-badge]

The [flux-cluster.yaml] blueprint describes a flux-framework cluster where flux
is deployed as the native resource manager.

See [README](../community/examples/flux-framework/README.md)

[flux-cluster.yaml]: ../community/examples/flux-framework/flux-cluster.yaml

### [hpc-slurm-sharedvpc.yaml] ![community-badge] ![experimental-badge]

This blueprint demonstrates the use of the Slurm and Filestore modules in
the service project of an existing Shared VPC. Before attempting to deploy the
blueprint, one must first complete [initial setup for provisioning Filestore in
a Shared VPC service project][fs-shared-vpc]. Depending on how the shared VPC
was created one may have to perform a few additional manual steps to configure
the VPC. One may need to create firewall rules allowing SSH to be able to access
the controller and login nodes. Also since this blueprint doesn't use external
IPs for compute nodes, one must needs to [set up cloud nat][cloudnat] and
[set up iap][iap].

Now, one needs to update the blueprint to include shared vpc details. In the
network configuration, update the details for shared vpc as mentioned below,

```yaml
vars:
  project_id:  <service-project> # update /w the service project id in which shared network will be used.
  host_project_id: <host-project> # update /w the host project id in which shared network is created.
  deployment_name: hpc-small-shared-vpc
  region: us-central1
  zone: us-central1-c

deployment_groups:
- group: primary
  modules:
  - id: network1
    source: modules/network/pre-existing-vpc
    settings:
      project_id: $(vars.host_project_id)
      network_name: <shared-network> # update /w shared network name
      subnetwork_name: <shared-subnetwork> # update /w shared sub-net name
```

[hpc-slurm-sharedvpc.yaml]: ../community/examples/hpc-slurm-sharedvpc.yaml
[fs-shared-vpc]: https://cloud.google.com/filestore/docs/shared-vpc

### [gke-tpu-v6] ![community-badge] ![experimental-badge]

This example shows how TPU v6 cluster can be created and be used to run a job that requires TPU capacity on GKE. Additional information on TPU blueprint and associated changes are in this [README](/community/examples/gke-tpu-v6/README.md).

[gke-tpu-v6]: ../community/examples/gke-tpu-v6

### [xpk-n2-filestore] ![community-badge] ![experimental-badge]

This example shows how to set up an [XPK](https://github.com/AI-Hypercomputer/xpk)-compatible GKE cluster - giving researchers a Slurm-like CLI experience but with lightweight Kueue and Kjob resources on the cluster side. The blueprint creates a low-cost, CPU-based XPK cluster, using a single n2-standard-32-2 slice.

Client-side installation of the XPK CLI is also required (see the [prerequisites](https://github.com/AI-Hypercomputer/xpk?tab=readme-ov-file#prerequisites) and [installation](https://github.com/AI-Hypercomputer/xpk?tab=readme-ov-file#installation) in the XPK repository). Set `gcloud config set compute/zone <zone>` and `gcloud config set project <project-id>` to avoid their repeated inclusion in XPK commands.

Attach the Filestore instance for use in workloads and jobs with the `xpk storage` command:

```bash
python3 xpk.py storage attach xpk-01-homefs \
  --cluster=xpk-01 \
  --type=gcpfilestore \
  --auto-mount=true \
  --mount-point=/home \
  --mount-options="" \
  --readonly=false \
  --size=1024 \
  --vol=nfsshare
```

After blueprint provisioning, XPK CLI installation, and storage setup, users can run interactive shells, workloads, and jobs:

```bash
# Start an interactive shell (somewhat analogous to a Slurm login node)
python3 xpk.py shell --cluster xpk-01
```

```bash
# Submit a workload (Kueue-based)
python3 xpk.py workload create \
  --cluster xpk-01 \
  --num-slices=1 \
  --device-type=n2-standard-32-2 \
  --workload xpk-test-workload \
  --command="ls /home"
```

```bash
# Run and manage jobs (kjob-focused)
python3 xpk.py run --cluster xpk-01 your-script.sh
python3 xpk.py batch --cluster xpk-01 your-script.sh
python3 xpk.py info --cluster xpk-01
```

[xpk-n2-filestore]: ../community/examples/xpk-n2-filestore/xpk-n2-filestore.yaml

### [gke-h4d] ![core-badge]

This blueprint uses GKE to provision a Kubernetes cluster and a H4D node pool, along with networks and service accounts. Information about H4D machines can be found [here](https://cloud.google.com/blog/products/compute/new-h4d-vms-optimized-for-hpc). The deployment instructions can be found in the [README](/examples/gke-h4d/README.md).

[gke-h4d]: ../examples/gke-h4d

### [gke-g4] ![core-badge]

This blueprint uses GKE to provision a Kubernetes cluster and a G4 node pool, along with networks and service accounts. Information about G4 machines can be found [here](https://cloud.google.com/blog/products/compute/introducing-g4-vm-with-nvidia-rtx-pro-6000). The deployment instructions can be found in the [README](/examples/gke-g4/README.md).

[gke-g4]: ../examples/gke-g4

## Blueprint Schema

Similar documentation can be found on
[Google Cloud Docs](https://cloud.google.com/cluster-toolkit/docs/setup/hpc-blueprint).

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
  # Embedded module (part of the toolkit), prefixed with `modules/` or `community/modules`
  - id: <a unique id> # Required: Name of this module used to uniquely identify it.
    source: modules/role/module-name # Required
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

  # GitHub module over SSH, prefixed with git@github.com
  - source: git@github.com:org/repo.git//path/to/module

  # GitHub module over HTTPS, prefixed with github.com
  - source: github.com/org/repo//path/to/module

  # Local absolute source, prefixed with /
  - source: /path/to/module

  # Local relative (to current working directory) source, prefixed with ./ or ../
  - source: ../path/to/module
  # NOTE: Do not reference toolkit modules by local source, use embedded source instead.
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
toolkit_modules_url: # github.com/GoogleCloudPlatform/cluster-toolkit
toolkit_modules_version: # v1.38.0

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

* **toolkit_modules_url** and **toolkit_modules_version** (optional): The blueprint schema provides the optional fields `toolkit_modules_url` and `toolkit_modules_version` to version a blueprint. When these fields are provided, any module in the blueprint with a reference to an embedded module in its source field will be updated to reference the specified GitHub source and toolkit version in the deployment folder. `toolkit_modules_url` specifies the base URL of the GitHub repository containing the modules and `toolkit_modules_version` specifies the version of the modules to use. `toolkit_modules_url` and `toolkit_modules_version` should be provided together when in use.

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

The Cluster Toolkit uses special reserved labels for monitoring each deployment.
These are set automatically, but can be overridden in vars or module settings.
They include:

* ghpc_blueprint: The name of the blueprint the deployment was created from
* ghpc_deployment: The name of the specific deployment
* ghpc_role: See below

A module role is a default label applied to modules (`ghpc_role`), which
conveys what role that module plays within a larger HPC environment.

The modules provided with the Cluster Toolkit have been divided into roles
matching the names of folders in the [modules/](../modules/) and
[community/modules](../community/modules/) directories (compute,
file-system etc.).

When possible, custom modules should use these roles so that they match other
modules defined by the toolkit. If a custom module does not fit into these
roles, a new role can be defined.

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

## Variables, expressions and functions

Variables can be used to refer both to values defined elsewhere in the blueprint
and to the output and structure of other modules.

> [!NOTE]
> "Brackets-less" access to elements of collection is not supported, use brackets.
> E.g. `pink.lime[0].salmon` instead of `pink.lime.0.salmon`.

### Blueprint expressions

Expressions in a blueprint file can refer to deployment variables or the outputs
of other modules. The expressions can only be used within `vars`, module `settings`, and `terraform_backend` blocks.
The entire expression is wrapped in `$()`, the syntax is as follows:

```yaml
vars:
  zone: us-central1-a
  num_nodes: 2

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
            # access nested fields
            key3: $(resource1.nodes[0].private_ip)
            # arithmetic expression
            key4: $(vars.num_nodes + 5)
            # string interpolation
            key5: $(resource1.name)_$(vars.zone)
            # multiline string interpolation
            key6: |
              #!/bin/bash
              echo "Hello $(vars.project_id) from $(vars.region)"
            # use a function, supported by Terraform
            key7: $(jsonencode(resource1.config))
```

#### Escape expressions

Under circumstances where the expression notation conflicts with the content of a setting or string, for instance when defining a startup-script runner that uses a subshell like in the example below, a non-quoted backslash (`\`) can be used as an escape character. It preserves the literal value of the next character that follows:  `\$(not.bp_var)` evaluates to `$(not.bp_var)`.

```yaml
deployment_groups:
  - group: primary
     modules:
       - id: resource1
         source: path/to/module/1
         settings:
            key1: |
              #!/bin/bash
              echo \$(cat /tmp/file1)    ## Evaluates to "echo $(cat /tmp/file1)"
```

### Functions

Blueprint supports a number of functions that can be used within expressions to manipulate variables:

* `merge`, `flatten` - same as Terraform's functions with the same name;
* `ghpc_stage` - copy referenced file to the deployment directory;

The expressions in `settings`-block of Terraform modules can additionally use any functions available in Terraform.

#### `ghpc_stage`

Using local files in the blueprint can be challenging, relative paths may become invalid relatevly to deployment directory, or
deployment directory can get moved to another machine.

To avoid these issues, the `ghpc_stage` function can be used to copy a file (or whole directory) to the deployment directory. The returned value is the path to the staged file relative to the root of deployment group directory.

```yaml
  ...
  - id: script
    source: modules/scripts/startup-script
    settings:
      runners:
      - type: shell
        destination: hi.sh
        source: $(ghpc_stage("path/relative/to/blueprint/hi.sh"))
        # or stage the whole directory
        source: $(ghpc_stage("path"))/hi.sh
        # or use it as input to another function
        content: $(file(ghpc_stage("path/hi.sh")))
```

The `ghpc_stage` function will always look first in the path specified in the blueprint. If the file is not found at this path then `ghpc_stage` will look for the staged file in the deployment folder, if a deployment folder exists.
This means that you can redeploy a blueprint (`gcluster deploy <blueprint> -w`) so long as you have the deployment folder from the original deployment, even if locally referenced files are not available.

## Completed Migration to Slurm-GCP v6

Slurm-GCP v5 users should read [Slurm-GCP v5 EOL](../docs/slurm-gcp-support.md)
for information on v5 retirement and feature highlights for v6. Slurm-GCP v6 is
only supported option within the Toolkit.
