# Example Blueprints

> [!NOTE]
> Migration to Slurm-GCP v6 is completed. See
> [this update](#ongoing-migration-to-slurm-gcp-v6) for specific recommendations
> and timelines.

This directory contains a set of example blueprint files that can be fed into
gHPC to create a deployment.

<!-- TOC generated with some manual tweaking of the following command output:
md_toc github examples/README.md | sed -e "s/\s-\s/ * /"
-->
<!-- TOC -->

* [Instructions](#instructions)
  * [(Optional) Setting up a remote terraform state](#optional-setting-up-a-remote-terraform-state)
* [Ongoing Migration to Slurm-GCP v6](#ongoing-migration-to-slurm-gcp-v6)
* [Blueprint Descriptions](#blueprint-descriptions)
  * [hpc-slurm-v5-legacy.yaml](#hpc-slurm-v5-legacyyaml-) ![core-badge]
  * [hpc-slurm.yaml](#hpc-slurmyaml-) ![core-badge]
  * [hpc-enterprise-slurm-v5-legacy.yaml](#hpc-enterprise-slurm-v5-legacyyaml-) ![core-badge]
  * [hpc-enterprise-slurm.yaml](#hpc-enterprise-slurmyaml-) ![core-badge]
  * [hpc-slurm-static.yaml](#hpc-slurm-staticyaml-) ![core-badge]
  * [hpc-slurm6-tpu.yaml](#hpc-slurm6-tpuyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm6-tpu-maxtext.yaml](#hpc-slurm6-tpu-maxtextyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm6-apptainer.yaml](#hpc-slurm6-apptaineryaml--) ![community-badge] ![experimental-badge]
  * [ml-slurm-v5-legacy.yaml](#ml-slurm-v5-legacyyaml-) ![core-badge]
  * [ml-slurm.yaml](#ml-slurmyaml-) ![core-badge]
  * [image-builder-v5-legacy.yaml](#image-builder-v5-legacyyaml-) ![core-badge]
  * [image-builder.yaml](#image-builderyaml--) ![core-badge]
  * [serverless-batch.yaml](#serverless-batchyaml-) ![core-badge]
  * [serverless-batch-mpi.yaml](#serverless-batch-mpiyaml-) ![core-badge]
  * [pfs-lustre.yaml](#pfs-lustreyaml-) ![core-badge]
  * [cae-slurm-v5-legacy.yaml](#cae-slurm-v5-legacyyaml-) ![core-badge]
  * [cae-slurm.yaml](#cae-slurmyaml-) ![core-badge]
  * [hpc-build-slurm-image.yaml](#hpc-build-slurm-imageyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-ubuntu2004-v5-legacy.yaml](#hpc-slurm-ubuntu2004-v5-legacyyaml-) ![community-badge]
  * [hpc-slurm-ubuntu2004.yaml](#hpc-slurm-ubuntu2004yaml--) ![community-badge]
  * [pfs-daos.yaml](#pfs-daosyaml-) ![community-badge]
  * [hpc-slurm-daos.yaml](#hpc-slurm-daosyaml-) ![community-badge]
  * [hpc-amd-slurm-v5-legacy.yaml](#hpc-amd-slurm-v5-legacyyaml-) ![community-badge]
  * [hpc-amd-slurm.yaml](#hpc-amd-slurmyaml-) ![community-badge]
  * [hpc-slurm-sharedvpc.yaml](#hpc-slurm-sharedvpcyaml--) ![community-badge] ![experimental-badge]
  * [client-google-cloud-storage.yaml](#client-google-cloud-storageyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-gromacs.yaml](#hpc-slurm-gromacsyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-local-ssd-v5-legacy.yaml](#hpc-slurm-local-ssd-v5-legacyyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-local-ssd.yaml](#hpc-slurm-local-ssdyaml--) ![community-badge] ![experimental-badge]
  * [hcls-blueprint.yaml](#hcls-blueprintyaml-) ![core-badge]
  * [hpc-gke.yaml](#hpc-gkeyaml--) ![community-badge] ![experimental-badge]
  * [ml-gke](#ml-gkeyaml--) ![community-badge] ![experimental-badge]
  * [storage-gke](#storage-gkeyaml--) ![community-badge] ![experimental-badge]
  * [htc-slurm-v5-legacy.yaml](#htc-slurm-v5-legacyyaml--) ![community-badge] ![experimental-badge]
  * [htc-slurm.yaml](#htc-slurmyaml-) ![community-badge]
  * [htc-htcondor.yaml](#htc-htcondoryaml--) ![community-badge] ![experimental-badge]
  * [fsi-montecarlo-on-batch.yaml](#fsi-montecarlo-on-batchyaml-) ![community-badge] ![experimental-badge]
  * [tutorial-starccm-slurm.yaml](#tutorial-starccm-slurmyaml--) ![community-badge] ![experimental-badge]
  * [tutorial-starccm.yaml](#tutorial-starccmyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-ramble-gromacs.yaml](#hpc-slurm-ramble-gromacsyaml--) ![community-badge] ![experimental-badge]
  * [hpc-slurm-chromedesktop-v5-legacy.yaml](#hpc-slurm-chromedesktop-v5-legacyyaml--) ![community-badge] ![experimental-badge]
  * [flux-cluster](#flux-clusteryaml--) ![community-badge] ![experimental-badge]
  * [tutorial-fluent.yaml](#tutorial-fluentyaml--) ![community-badge] ![experimental-badge]
  * [omnia-cluster.yaml](#omnia-clusteryaml---) ![community-badge] ![experimental-badge] ![deprecated-badge]
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

There are two ways to specify [terraform backends] in HPC Toolkit: a default setting that propagates all groups and custom per-group configuration:

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
./ghpc create examples/hpc-slurm.yaml \
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

## Ongoing Migration to Slurm-GCP v6

[Slurm-GCP](https://github.com/GoogleCloudPlatform/slurm-gcp) is the set of
scripts and tools that automate the installation, deployment, and certain
operational aspects of [Slurm](https://slurm.schedmd.com/overview.html) on
Google Cloud Platform. It is recommended to use Slurm-GCP through the HPC
toolkit where it is exposed as various modules.

The HPC Toolkit team has finished transitioning from Slurm-GCP v5 to Slurm-GCP v6 and
now Slurm-GCP v6 is the recommended option. Following this, blueprint naming would be
as follows:

* Slurm v5: hpc-slurm-v5-legacy.yaml
* Slurm v6: hpc-slurm.yaml

> [!IMPORTANT]
> Three months after Slurm-gcp V6 becomes the recommended version, Slurm v5
> modules will be marked as deprecated and will be maintained in our repo for
> another three months, at which point the modules will be removed from the HPC
> Toolkit repo and regression tests will no longer run for V5. Those who choose
> to not upgrade to V6 will still be able to use V5 modules by referencing
> specific git tags in the module source lines.

### Major Changes in from Slurm GCP v5 to v6

* Robust reconfiguration

  Reconfiguration is now managed by a service that runs on each instance. This has removed the dependency on the Pub/Sub Google cloud service, and provides a more consistent reconfiguration experience (when calling `ghpc deploy blueprint.yaml -w`). Reconfiguration has also been enabled by default.

* Faster deployments

  Simple cluster deploys up to 3x faster.

* Lift the restriction on the number of deployments in a single project.

  Slurm GCP v6 has eliminated the use of project metadata to store cluster configuration. Project metadata was both slow to update and had an absolute storage limit. This restricted the number of clusters that could be deployed in a single project. Configs are now stored in a Google Storage Bucket.

* Fewer dependencies in the deployment environment

  Reconfiguration and compute node cleanup no longer require users to install local python dependencies in the deployment environment (where ghpc is called). This has allowed for these features to be enabled by default.

* Flexible node to partition relation

  The v5 concept of "node-group" was replaced by "nodeset" to align with Slurm naming convention. Nodeset can be attributed to multiple partitions, as well as partitions can include multiple nodesets.

* Upgrade Slurm to 23.11
* TPU v3, v4 support

_For a full accounting of changes, see the changelog._

## Blueprint Descriptions

[core-badge]: https://img.shields.io/badge/-core-blue?style=plastic
[community-badge]: https://img.shields.io/badge/-community-%23b8def4?style=plastic
[stable-badge]: https://img.shields.io/badge/-stable-lightgrey?style=plastic
[experimental-badge]: https://img.shields.io/badge/-experimental-%23febfa2?style=plastic
[deprecated-badge]: https://img.shields.io/badge/-deprecated-%23fea2a2?style=plastic

The example blueprints listed below labeled with the core badge
(![core-badge]) are located in this folder and are developed and tested by the
HPC Toolkit team directly.

The community blueprints are contributed by the community (including the HPC
Toolkit team, partners, etc.) and are labeled with the community badge
(![community-badge]). The community blueprints are located in the
[community folder](../community/examples/).

Blueprints that are still in development and less stable are also labeled with
the experimental badge (![experimental-badge]).

### [hpc-slurm-v5-legacy.yaml] ![core-badge]

> **Warning**: The variables `enable_reconfigure`,
> `enable_cleanup_compute`, and `enable_cleanup_subscriptions`, if set to
> `true`, require additional dependencies **to be installed on the system deploying the infrastructure**.
>
> ```shell
> # Install Python3 and run
> pip3 install -r https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/5.11.1/scripts/requirements.txt
> ```

Creates a basic auto-scaling Slurm cluster with mostly default settings. The
blueprint also creates a new VPC network, and a filestore instance mounted to
`/home`.

There are 3 partitions in this example: `debug` `compute`, and `h3`. The `debug`
partition uses `n2-standard-2` VMs, which should work out of the box without
needing to request additional quota. The purpose of the `debug` partition is to
make sure that first time users are not immediately blocked by quota
limitations.

[hpc-slurm-v5-legacy.yaml]: ./hpc-slurm-v5-legacy.yaml

#### Compute Partition

There is a `compute` partition that achieves higher performance. Any
performance analysis should be done on the `compute` partition. By default it
uses `c2-standard-60` VMs with placement groups enabled. You may need to request
additional quota for `C2 CPUs` in the region you are deploying in. You can
select the compute partition using the `-p compute` argument when running `srun`.

#### H3 Partition

There is an `h3` partition that uses compute-optimized `h3-standard-88` machine type.
You can read more about the H3 machine series [here](https://cloud.google.com/compute/docs/compute-optimized-machines#h3_series).

#### Quota Requirements for hpc-slurm-v5-legacy.yaml

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

### [hpc-enterprise-slurm-v5-legacy.yaml] ![core-badge]

This advanced blueprint creates a cluster with Slurm with several performance
tunings enabled, along with tiered file systems for higher performance. Some of
these features come with additional cost and required additional quotas.

The Slurm system deployed here connects to the default VPC of the project and
creates a  login node and the following seven partitions:

* `n2` with general-purpose [`n2-stardard-2` nodes][n2]. Placement policies and
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

> **Warning**: The DDN Exascaler Lustre file system has a license cost as
> described in the pricing section of the
> [DDN EXAScaler Cloud Marketplace Solution](https://console.developers.google.com/marketplace/product/ddnstorage/).

#### Quota Requirements for hpc-enterprise-slurm-v5-legacy.yaml

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

[hpc-enterprise-slurm-v5-legacy.yaml]: ./hpc-enterprise-slurm-v5-legacy.yaml

### [hpc-enterprise-slurm.yaml] ![core-badge]

This advanced blueprint creates a cluster with Slurm with several performance
tunings enabled, along with tiered file systems for higher performance. Some of
these features come with additional cost and required additional quotas.

The Slurm system deployed here connects to the default VPC of the project and
creates a  login node and the following seven partitions:

* `n2` with general-purpose [`n2-stardard-2` nodes][n2]. Placement policies and
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

> **Warning**: The DDN Exascaler Lustre file system has a license cost as
> described in the pricing section of the
> [DDN EXAScaler Cloud Marketplace Solution](https://console.developers.google.com/marketplace/product/ddnstorage/).

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

* [About [Slurm] node types](https://cloud.google.com/hpc-toolkit/docs/slurm/node-types)
* [Best practices for static compute nodes]
* [Reconfigure a running cluster](http://cloud/hpc-toolkit/docs/slurm/reconfigure-cluster)
* [Manage static compute nodes](http://cloud/hpc-toolkit/docs/slurm/manage-static-nodes)

For a similar, more advanced, example which demonstrates static node
functionality with GPUs, see the
[ML Slurm A3 example](./machine-learning/README.md).

[Best practices for static compute nodes]: http://cloud/hpc-toolkit/docs/slurm/static-nodes-best-practices
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

### [ml-slurm-v5-legacy.yaml] ![core-badge]

This blueprint provisions an HPC cluster running the Slurm scheduler with the
machine learning frameworks PyTorch and TensorFlow pre-installed on every
VM. The cluster has 2 partitions:

* [A2 family VMs][a2] with the NVIDIA A100 GPU accelerator
* [G2 family VMs][g2] with the NVIDIA L4 GPU accelerator

[a2]: https://cloud.google.com/compute/docs/gpus#a100-gpus
[g2]: https://cloud.google.com/compute/docs/gpus#l4-gpus

To provision the cluster, please run:

```text
./ghpc create examples/ml-slurm-v5-legacy.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}"
./ghpc deploy ml-example
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
sbatch -N 1 torch_test.sh
```

When you are done, clean up the resources in reverse order of creation:

```text
./ghpc destroy ml-example
```

Finally, browse to the [Cloud Console][console-images] to delete your custom
image. It will be named beginning with `ml-slurm` followed by a date and
timestamp for uniqueness.

[ml-slurm-v5-legacy.yaml]: ../examples/ml-slurm-v5-legacy.yaml

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
./ghpc create examples/ml-slurm.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}"
./ghpc deploy ml-example-v6
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
sbatch -N 1 torch_test.sh
```

When you are done, clean up the resources in reverse order of creation:

```text
./ghpc destroy ml-example-v6
```

Finally, browse to the [Cloud Console][console-images] to delete your custom
image. It will be named beginning with `ml-slurm` followed by a date and
timestamp for uniqueness.

[ml-slurm.yaml]: ../examples/ml-slurm.yaml

### [image-builder-v5-legacy.yaml] ![core-badge]

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
./ghpc create examples/image-builder-v5-legacy.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}"
./ghpc deploy image-builder-001"
```

Follow the on-screen prompts to approve the creation of each deployment group.
For example, the network is created in the first deployment group, the VM image
is created in the second group, and the third group uses the image to create an
HPC cluster using the Slurm scheduler.

When you are done, clean up the resources in reverse order of creation:

```text
terraform -chdir=image-builder-001/cluster destroy --auto-approve
terraform -chdir=image-builder-001/primary destroy --auto-approve
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
[image-builder-v5-legacy.yaml]: ./image-builder-v5-legacy.yaml

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

#### Quota Requirements for image-builder-v5-legacy.yaml

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
./ghpc create examples/image-builder.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}"
./ghpc deploy image-builder-v6-001"
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

This example demonstrates how to use the HPC Toolkit to set up a Google Cloud Batch job
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

### [pfs-lustre.yaml] ![core-badge]

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

#### Quota Requirements for pfs-lustre.yaml

For this example the following is needed in the selected region:

* Compute Engine API: Persistent Disk SSD (GB): **~14TB: 3500GB MDT, 3500GB OST[0-2]**
* Compute Engine API: Persistent Disk Standard (GB): **~756GB: 20GB MDS, 276GB MGS, 3x20GB OSS, 2x200GB client-vms**
* Compute Engine API: N2 CPUs: **~116: 32 MDS, 32 MGS, 3x16 OSS, 2x2 client-vms**

[pfs-lustre.yaml]: ./pfs-lustre.yaml

### [cae-slurm-v5-legacy.yaml] ![core-badge]

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

#### Quota Requirements for cae-slurm-v5-legacy.yaml

For this example the following is needed in the selected region:

* Cloud Filestore API: Basic SSD capacity (GB) per region: **5,120 GB**
* Cloud Filestore API: High Scale SSD capacity (GB) per region: **10,240 GB**
* Compute Engine API: H3 CPUs: **88/node** active in `balance` partition up to 880
* Compute Engine API: C3-highmem CPUs: **176/node** active in `highmem` partition up to 1,760
* Compute Engine API: N1 CPUs: **8/node** active in `desktop` partition up to 40
* Compute Engine API: T4 GPUs: **1/node** active in `desktop` partition up to 5
* Compute Engine API: N2 CPUs: **8** for login and **16** for controller

[cae-slurm-v5-legacy.yaml]: ../examples/cae/cae-slurm-v5-lgacy.yaml

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

This blueprint demonstrates how to use HPC Toolkit to build a Slurm image on top
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

### [hpc-slurm-ubuntu2004-v5-legacy.yaml] ![community-badge]

> **Warning**: The variables `enable_reconfigure`,
> `enable_cleanup_compute`, and `enable_cleanup_subscriptions`, if set to
> `true`, require additional dependencies **to be installed on the system deploying the infrastructure**.
>
> ```shell
> # Install Python3 and run
> pip3 install -r https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/5.11.1/scripts/requirements.txt
> ```

Similar to the [hpc-slurm-v5-legacy.yaml] example, but using Ubuntu 20.04 instead of CentOS 7.
[Other operating systems] are supported by SchedMD for the the Slurm on GCP project and images are listed [here](https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/docs/images.md#published-image-family). Only the examples listed in this page been tested by the Cloud HPC Toolkit team.

The cluster will support 2 partitions named `debug` and `compute`.
The `debug` partition is the default partition and runs on smaller
`n2-standard-2` nodes. The `compute` partition is not default and requires
specifying in the `srun` command via the `--partition` flag. The `compute`
partition runs on compute optimized nodes of type `cs-standard-60`. The
`compute` partition may require additional quota before using.

[Other operating systems]: https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/docs/images.md#supported-operating-systems
[hpc-slurm-ubuntu2004-v5-legacy.yaml]: ../community/examples/hpc-slurm-ubuntu2004-v5-legacy.yaml

#### Quota Requirements for hpc-slurm-ubuntu2004-v5-legacy.yaml

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

### [hpc-slurm-ubuntu2004.yaml] ![community-badge]

Similar to the [hpc-slurm.yaml] example, but using Ubuntu 20.04 instead of CentOS 7.
[Other operating systems] are supported by SchedMD for the the Slurm on GCP project and images are listed [here](https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/docs/images.md#published-image-family). Only the examples listed in this page been tested by the Cloud HPC Toolkit team.

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

### [pfs-daos.yaml] ![community-badge]

This example provisions a DAOS cluster with [managed instance groups][migs] for the servers and for clients. It is more extensively discussed in a dedicated [README for Intel
examples][intel-examples-readme].

[pfs-daos.yaml]: ../community/examples/intel/pfs-daos.yaml
[migs]: https://cloud.google.com/compute/docs/instance-groups

### [hpc-slurm-daos.yaml] ![community-badge]

This example provisions DAOS servers and a Slurm cluster. It is
more extensively discussed in a dedicated [README for Intel
examples][intel-examples-readme].

[hpc-slurm-daos.yaml]: ../community/examples/intel/hpc-slurm-daos.yaml

### [hpc-amd-slurm-v5-legacy.yaml] ![community-badge]

This example provisions a Slurm cluster using AMD VM machine types. It
automates the initial setup of Spack, including a script that can be used to
install the AMD Optimizing C/C++ Compiler ([AOCC]) and compile OpenMPI with
AOCC. It is more extensively discussed in a dedicated [README for AMD
examples][amd-examples-readme].

[hpc-amd-slurm-v5-legacy.yaml]: ../community/examples/AMD/hpc-amd-slurm-v5-legacy.yaml
[AOCC]: https://developer.amd.com/amd-aocc/
[amd-examples-readme]: ../community/examples/AMD/README.md

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
(GCS) buckets in the HPC Toolkit. There are two buckets referenced in the
example:

1. A GCS bucket that is created by the HPC Toolkit (`id: new-bucket`).
1. A GCS bucket that is created externally from the HPC Toolkit but referenced
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
./ghpc create community/examples/client-google-cloud-storage.yaml \
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

### [omnia-cluster.yaml] ![community-badge] ![experimental-badge] ![deprecated-badge]

_This blueprint has been deprecated and will be removed on August 1, 2024._

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

### [hpc-slurm-local-ssd-v5-legacy.yaml] ![community-badge] ![experimental-badge]

This blueprint demonstrates the use of Slurm and Filestore, with the definition
of a partition which deploys compute nodes that have local ssd drives deployed.
Before deploying this blueprint, one must first ensure to have an existing VPC
properly configured (allowing Internet access and allowing inter virtual
machine communications, for NFS and also for communications between the Slurm
nodes)

[hpc-slurm-local-ssd-v5-legacy.yaml]: ../community/examples/hpc-slurm-local-ssd-v5-legacy.yaml

### [hpc-slurm-local-ssd.yaml] ![community-badge] ![experimental-badge]

This blueprint demonstrates the use of Slurm and Filestore, with compute nodes
that have local ssd drives deployed.

[hpc-slurm-local-ssd.yaml]: ../community/examples/hpc-slurm-local-ssd.yaml

### [hcls-blueprint.yaml]: ![core-badge]

This blueprint demonstrates an advanced architecture that can be used to run
GROMACS with GPUs and CPUs on Google Cloud. For full documentation, refer
[document].

[document]: ../docs/videos/healthcare-and-life-sciences/README.md
[hcls-blueprint.yaml]:  ../example/hcls-blueprint.yaml

### [hpc-gke.yaml] ![community-badge] ![experimental-badge]

This blueprint uses GKE to provision a Kubernetes cluster with a system node
pool (included in gke-cluster module) and an autoscaling compute node pool. It
creates a VPC configured to be used by a VPC native GKE cluster with subnet
secondary IP ranges defined.

The `gke-job-template` module is used to create a job file that can be submitted
to the cluster using `kubectl` and will run on the specified node pool.

[hpc-gke.yaml]: ../community/examples/hpc-gke.yaml

### [ml-gke.yaml] ![community-badge] ![experimental-badge]

This blueprint demonstrates how to set up a GPU GKE cluster using the HPC
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
  source: community/modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      disk_type: pd-balanced
      machine_type: a2-highgpu-2g
  ```

  Users only need to provide machine type for standard ["a2", "a3" and "g2"] machine families,
  while the other settings like `type`, `count` , `gpu_driver_installation_config` will default to
  machine family specific values.
  However, for other standard or custom machine families users will need to provide
  the entire configuration as follows:

```yaml
machine_type: n1-standard-1
guest_accelerator:
- type: nvidia-tesla-t4
  count: 1
  gpu_partition_size: null
  gpu_sharing_config: null
  gpu_driver_installation_config:
  - gpu_driver_version: "DEFAULT"
```

Custom g2 pool

```yaml
machine_type: g2-custom-16-55296
guest_accelerator:
- type: nvidia-l4
  count: 1
  gpu_partition_size: null
  gpu_sharing_config:
  -  max_shared_clients_per_gpu: 2
      gpu_sharing_strategy: "TIME_SHARING"
  gpu_driver_installation_config:
  - gpu_driver_version: "LATEST"
```

* Configuration of the cluster using default drivers provided by GKE.
* Creation of a job template yaml file that can be used to submit jobs to the
  GPU node pool.

> **Note**: The Kubernetes API server will only allow requests from authorized
> networks. Nvidia drivers are installed on GPU nodes by a DaemonSet created by
> the [`kubernetes-operations`] Terraform module. **You must use the
> `authorized_cidr` variable to supply an authorized network which contains the
> IP address of the machine deploying the blueprint, for example
> `--vars authorized_cidr=<your-ip-address>/32`.** This will allow Terraform to
> create the necessary DaemonSet on the cluster. You can use a service like
> [whatismyip.com](https://whatismyip.com) to determine your IP address.

Once you have deployed the blueprint, follow output instructions to _fetch
credentials for the created cluster_ and _submit a job calling `nvidia_smi`_.

[ml-gke.yaml]: ../community/examples/ml-gke.yaml
[`kubernetes-operations`]: ../community/modules/scripts/kubernetes-operations/README.md

### [storage-gke.yaml] ![community-badge] ![experimental-badge]

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

[storage-gke.yaml]: ../community/examples/storage-gke.yaml

### [htc-htcondor.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions an auto-scaling [HTCondor][htcondor] pool based upon
the [HPC VM Image][hpcvmimage].

Also see the [tutorial](../docs/tutorials/README.md#htcondor-tutorial), which
walks through the use of this blueprint.

[htcondor]: https://htcondor.org/
[htc-htcondor.yaml]: ../community/examples/htc-htcondor.yaml
[hpcvmimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm

### [htc-slurm-v5-legacy.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions a cluster using the Slurm scheduler in a configuration
tuned for the execution of many short-duration, loosely-coupled (non-MPI) jobs.

For more information see:

* [Slurm on Google Cloud High Throughput documentation](https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/docs/htc.md)
* [General Slurm High Throughput documentation](https://slurm.schedmd.com/high_throughput.html)

[htc-slurm-v5-legacy.yaml]: ../community/examples/htc-slurm-v5-legacy.yaml

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

> The main tutorial is described on the [HPC Toolkit website](https://cloud.google.com/hpc-toolkit/docs/simcenter-starccm-slurm/run-workload).

[tutorial-starccm-slurm.yaml]: ../community/examples/tutorial-starccm-slurm.yaml

### [tutorial-starccm.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions a simple cluster for use with a Simcenter StarCCM+
tutorial.

> The main tutorial is described on the [HPC Toolkit website](https://cloud.google.com/hpc-toolkit/docs/simcenter-star-ccm/run-workload).

[tutorial-starccm.yaml]: ../community/examples/tutorial-starccm.yaml

### [tutorial-fluent.yaml] ![community-badge] ![experimental-badge]

This blueprint provisions a simple cluster for use with an Ansys Fluent
tutorial.

> The main tutorial is described on the [HPC Toolkit website](https://cloud.google.com/hpc-toolkit/docs/tutorials/ansys-fluent).

[tutorial-fluent.yaml]: ../community/examples/tutorial-fluent.yaml

### [hpc-slurm-chromedesktop-v5-legacy.yaml] ![community-badge] ![experimental-badge]

This example shows how to use the `chrome-remote-desktop` module with a Slurm
partition to be able to `salloc` a GPU accelerated remote desktop.

After deploying the blueprint perform the following actions:
1. SSH to the Slurm login node or controller.
1. Provision a remote desktop with the following command: `salloc -p desktop -N
   1`
1. Once you see `salloc: Nodes slurmchrom-desktop-ghpc-0 are ready for job`,
   follow the [instructions to set up the remote desktop][crd-instructions].

[crd-instructions]: ../community/modules/remote-desktop/chrome-remote-desktop/README.md#setting-up-the-remote-desktop
[hpc-slurm-chromedesktop-v5-legacy.yaml]: ../community/examples/hpc-slurm-chromedesktop-v5-legacy.yaml
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

[hpc-slurm-sharedvpc.yaml]: ../community/examples/hpc-slurm-sharedvpc.yaml
[fs-shared-vpc]: https://cloud.google.com/filestore/docs/shared-vpc

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

## Variables, expressions and functions

Variables can be used to refer both to values defined elsewhere in the blueprint
and to the output and structure of other modules.

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
This means that you can redeploy a blueprint (`ghpc deploy <blueprint> -w`) so long as you have the deployment folder from the original deployment, even if locally referenced files are not available.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

No requirements.

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_compute_global_address.private_ip_alloc](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_global_address) | resource |
| [google-beta_google_compute_network.network](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_network) | resource |
| [google-beta_google_parallelstore_instance.instance](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_parallelstore_instance) | resource |
| [google-beta_google_service_networking_connection.default](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_service_networking_connection) | resource |

## Inputs

No inputs.

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_access_points"></a> [access\_points](#output\_access\_points) | Output access points |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
