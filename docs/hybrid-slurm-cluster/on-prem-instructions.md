# Setting up Hybrid Slurm Clusters Using the Cluster Toolkit

## Introduction

Cloud hybrid slurm clusters are slurm clusters that manage both local and cloud
partitions, where cloud partitions can elastically create resources in Google
Cloud as needed.

This document intends to support the use of the hybrid slurm terraform modules
provided by SchedMD via [Slurm on GCP][slurm-gcp] and are available in the Cluster
Toolkit through the [schedmd-slurm-gcp-v5-hybrid][hybridmodule] module.

> **_NOTE:_** Since on-premise Slurm configurations can vary significantly,
> it is likely that this document does not cover every edge case.
> The intent is to provide a useful starting point for setting up cloud
> hybrid partitions with Slurm, the Cluster Toolkit and Google Cloud.

## About the Hybrid Configuration Module

The [schedmd-slurm-gcp-v5-hybrid][hybridmodule] module creates the following:

* Compute instance templates that describe the nodes created by each cloud
  hybrid partition.
* Metadata in the Google Cloud project that informs newly created cloud compute
  resources how to configure themselves.
* Cloud pubsub triggers that handle reconfiguration and cleanup of cloud
  resources.
* Creating a configuration directory on the local filesystem with:
  * a `cloud.conf` file that extends the `slurm.conf` configuration.
  * a `cloud_gres.conf` file that extends the `gres.conf` configuration.
  * the [slurm-gcp] resume, suspend and synchronization scripts for
    the hybrid partitions.

This configuration comes with a set of assumptions about the local Slurm cluster
and cloud compute nodes. The following sections describe some of these in more
detail, as well as how to customize many of these assumptions to fit your needs.

[Slurm on GCP][slurm-gcp] provides additional documentation for hybrid
deployments in their [hybrid.md] documentation.

[hybridmodule]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md
[slurm-gcp]: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/5.12.2
[slurm\_controller\_hybrid]: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/master/terraform/slurm_cluster/modules/slurm_controller_hybrid
[hybrid.md]: https://github.com/GoogleCloudPlatform/slurm-gcp/blob/5.12.2/docs/hybrid.md

### NFS Mounts

The [slurm-gcp hybrid module][hybridmodule] depends on a set of NFS mounts to
provide slurm configuration information to recently created cloud compute nodes.
By default, the following directories will be directly mounted from the
associated slurm controller:

* `/usr/local/etc/slurm`
  * Required: **Yes**
  * Contains slurm configuration files such as `slurm.conf`.
* `/etc/munge`
  * Required: **Yes**
  * Contains the munge key file, `munge.key`.
* `/home`
  * Required: No, but recommended
  * Though not required, having the home directory shared between the controller
    and compute nodes ensure user data is available on the cloud compute nodes.
* `/opt/apps`
  * Required: No
  * Contains shared applications useful for running slurm jobs, such as open
    MPI.

By default, the compute node will fail to startup if any of these directories
fail to mount from the controller. For more information, see the
[Prepare NFS](#prepare-nfs) section.

### Slurm versions

The [slurm-gcp hybrid module][hybridmodule] only supports versions 21 and 22 of
Slurm on the cloud compute nodes. In addition, since the controller must be in
sync with Slurm version installed in the cloud compute nodes, the controller
must be upgraded to version 21 at minimum before making use of the
hybrid module.

The default Slurm cloud compute node images install version 22.05.3. If your
controller is running with slurm version 21, you can create a custom image with
your desired slurm version. For more information, see the section titled
[Creating a Slurm Compute Image](#creating-a-slurm-compute-image).

### User and Group IDs

The default [slurm-gcp] cloud compute VM disk images have the following user
and group IDs for the slurm and munge users and groups:

* `slurm`
  * User ID: 981
  * Group ID: 981
* `munge`
  * User ID: 980
  * Group ID: 980

If your Slurm controller sets different user and group IDs for these users, a
custom cloud compute VM disk image must be created with these values updated.
For more information and instructions, see
[Creating a Slurm Compute Image](#creating-a-slurm-compute-image).

### Power Saving Operations

The hybrid partitions rely on the Slurm [power saving][powersaving]
functionality for creation and tear down of the cloud compute VM instances. If
this functionality is also being used on your local slurm cluster, the
[slurm hybrid module][hybridmodule] is not be supported.

[powersaving]: https://slurm.schedmd.com/power_save.html

## Troubleshooting

For troubleshooting suggestions, visit [troubleshooting.md](./troubleshooting.md).

## Before You Begin

### Cloud Environment Setup

#### Select or Create a Google Cloud Project

This process will require a Google Cloud project where the cloud partitions will create
new compute VM instances to complete slurm jobs.

Identify or create the project you intend to use. For more information, visit
the Google Cloud documentation on
[Creating and Managing Projects](https://cloud.google.com/resource-manager/docs/creating-managing-projects)

#### Enable Required APIs

At minimum, the following APIs are required to complete these instructions:

* [Compute Engine API][computeapi]
* [Filestore API][fileapi]

[computeapi]: https://cloud.google.com/compute/docs/reference/rest/v1
[fileapi]: https://cloud.google.com/filestore/docs/reference/rest

#### Set IAM Roles
The authenticated service account used by the slurm controller must have the
Editor role in the Google Cloud project. For more information on authenticating
a service account, see the [Setup Authentication](#setup-authentication) section
and the Google Cloud documentation on
[Service Accounts](https://cloud.google.com/iam/docs/service-accounts).

#### Build gcluster

Before you begin, ensure that you have built the `gcluster` tool in the Cluster Toolkit.
For more information see the [README.md](../../README.md#quickstart) Quickstart.

The commands in these instructions assume the gcluster binary is installed in a
directory represented in the PATH environment variable. To ensure this is the
case, run `make install` after building `gcluster`:

```shell
make
make install
```

### Customize Your Blueprint

A valid Cluster Toolkit blueprint for creating a hybrid configuration deployment can
be found in the blueprints directory with the name [hybrid-configuration.yaml].
This blueprint can be customized to your needs, for example, partitions can be
updated or new partitions can be defined. See the documentation for the
[schedmd-slurm-gcp-v5-partition](../../community/modules/compute/schedmd-slurm-gcp-v5-partition/README.md)
module for more information.

Additionally, many of the parameters for the [schedmd-slurm-gcp-v5-hybrid][hybridmodule]
must be updated based on your Slurm controller environment. A few important
settings to be called out:

* [network_storage]: This is also used in [Prepare NFS](#prepare-nfs).
* [google_app_cred_path]: The path to the Service Account credentials on the
  Slurm controller. This allows you to authenticate as a Service Account using
  a key.
* [slurm_bin_dir]: The path to the slurm binaries, specifically `scontrol`. To
  determine this path, run `which scontrol`.
* [output_dir]: Location where the hybrid configuration will be created locally.
* [install_dir]: Intended location of installation of the hybrid configuration
  directory. These instructions assume the `install_dir` is in the same
  directory as the `slurm.conf` file.

[hybrid-configuration.yaml]: ./blueprints/hybrid-configuration.yaml
[network_storage]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md#input_network_storage
[google_app_cred_path]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md#input_google_app_cred_path
[slurm_bin_dir]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md#input_slurm_bin_dir
[output_dir]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md#input_output_dir
[install_dir]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md#input_install_dir

### Creating a Slurm Compute Image

The [Slurm on GCP][slurm-gcp] tool from SchedMD provides a set of default images
based on a selection of base VM disk images. In most cases when deploying a hybrid cluster,
many of the assumptions in these default images are not sufficient. Because of this, it
is recommended to create a custom image in your Google Cloud project based on the
requirements of your on-premise slurm cluster.

[Slurm on GCP][slurm-gcp] also provides a set of [packer templates][slurmgcppacker]
that can be used for creating a new image from scratch. The images can be
customized through the variables defined in [example.pkrvars.hcl], as well as
some of the variables defined in the ansible playbook that installs the
required software for the images.

The variables that are most relevant for the [slurm hybrid module][hybridmodule]
are listed below:

* [Slurm version][slurmversion]: Packer variable for setting a custom slurm
  version.
* [`service_account_scopes`]: Packer variable for attaching service account
  scopes to the cloud compute VM instances.
* [`munge_user`]: Ansible role variable for the munge user and group IDs.
* [`slurm_user`]: Ansible role variable for the slurm user and group IDs.

Remember to update your blueprint to include the `instance_image` in the
settings under your node groups. As an example, the
partition definition below sets the `project` to the `project_id`
in deployment variables and is using the default `family` for an
image created with slurm 21.08.8:

```yaml
- id: compute_node_group
    source: community/modules/compute/schedmd-slurm-gcp-v5-node-group
    settings:
      node_count_dynamic_max: 20
      instance_image:
        project: $(vars.project_id)
        family: slurm-gcp-5-12-hpc-centos-7

- id: compute-partition
  source: community/modules/compute/schedmd-slurm-gcp-v5-partition
  use:
  - network1
  - compute_node_group
  settings:
    partition_name: compute
```

[slurmgcppacker]: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/5.12.2/packer
[example.pkrvars.hcl]: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/5.12.2/packer/example.pkrvars.hcl
[slurmversion]: https://github.com/GoogleCloudPlatform/slurm-gcp/blob/5.12.2/packer/variables.pkr.hcl#L97
[`service_account_scopes`]: https://github.com/GoogleCloudPlatform/slurm-gcp/blob/5.12.2/packer/variables.pkr.hcl#L166
[`munge_user`]: https://github.com/GoogleCloudPlatform/slurm-gcp/blob/5.12.2/ansible/roles/munge/defaults/main.yml#L17
[`slurm_user`]: https://github.com/GoogleCloudPlatform/slurm-gcp/blob/5.12.2/ansible/roles/slurm/defaults/main.yml#L31

## On Premise Setup

### Networking

A VPN must be configured between the on-premise slurm cluster and the Google
Cloud project. For more information on setting up a Cloud VPN, see the
[Cloud VPN documentation][cloudvpn].

A DNS service must be configured between the on-premise slurm cluster and the
Google Cloud project. For more information on setting up a Cloud DNS, see the
[Cloud DNS documentation][clouddns].

[cloudvpn]: https://cloud.google.com/network-connectivity/docs/vpn/concepts/overview
[clouddns]: https://cloud.google.com/dns/docs/overview

### Install dependencies

#### Python
The [hybrid slurm module][hybridmodule] utilizes the
[Slurm power saving][powersaving] features by setting the resume and suspend
Programs to python scripts that create and delete cloud compute nodes
respectively. Because of this, python is required, as are a set of pip packages.

The required pip packages can be installed on the controller by running the
following command against the [requirements.txt] file in this directory:

```shell
sudo pip install -r docs/hybrid-slurm-cluster/requirements.txt
```

If pip is not present, it can be installed on centos, RHEL or Rocky environments
by running the following command:

```shell
sudo yum install python3-pip
```

And for Debian or Ubuntu distributions:

```shell
sudo apt-get install python3-pip
```

[requirements.txt]: ./requirements.txt

### Setup Authentication

If possible, [Workload Identity Federation][wif] is the preferred method of
authenticating on-premises resources with Google cloud. For more information
on configuring up Workload Identity Federation for your slurm controller, see
the [Google Cloud documentation][wifconfig].

An alternate, but less secure, authentication option is to use a service account
key for authentication. For more information, see the
[Google Cloud Docs][sakey].

For whichever authentication option is used, the `slurm` user needs to be
authenticated, as it will be the slurm user that performs the synchronization,
node creation and node deletion steps. This can be done by passing the path to
the credentials as the [`google_app_cred_path`][inputappcred] setting in the
[slurm-gcp hybrid module][hybridmodule].

[wif]: https://cloud.google.com/iam/docs/workload-identity-federation
[wifconfig]: https://cloud.google.com/iam/docs/configuring-workload-identity-federation
[sakey]: https://cloud.google.com/docs/authentication/provide-credentials-adc#local-key
[inputappcred]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md#input_google_app_cred_path

### Prepare NFS

For the four default mounted directories described in [NFS Mounts](#nfs-mounts),
one of three options must be used in order to successfully deploy a hybrid
partition from your on-premise slurm controller:

1. Use the default mounting behavior.
1. Create a new `network_storage` entry that mounts to the correct local path on
   the compute VM with a custom remote path or mounting options.
1. Mount from another source.

These options are described in more detail in the following sections.

The ability to bypass these mounting requirements will be coming in a future
iteration of the hybrid module.

> **_NOTE:_** The directories mounted from the controller must be exported by
> the controller and be visible to the cloud compute VM instances.

#### Default mounting behavior

You can use this option if:

* The path exists on the controller.
* The required contents exist in that location on the controller.

No explicit action is needed, this is the default behavior.

#### Create a New Network Storage Entry
If the path on the controller is not the same as is expected on the compute VMs
(see [NFS Mounts](#nfs-mounts) for more details), it is possible to override the
controller path that is mounted on the cloud compute VM at the expected
location.

Network storage is added as a list under the [`network_storage`][inputns]
setting of the [schedmd-slurm-gcp-v5-hybrid][hybridmodule] Cluster Toolkit Module.
An example showing how to do this with each of the default mount paths is
provided below:

```yaml
- id: slurm-controller
  source: community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid
  use:
  - debug-partition
  - compute-partition
  settings:
    output_dir: ./hybrid
    slurm_bin_dir: /usr/local/bin
    slurm_control_host: $(vars.static_controller_hostname)
    install_dir: /etc/slurm/hybrid
    network_storage:
    - server_ip: cluster-controller-0 # IP or hostname of the slurm controller
      remote_mount: /etc/slurm
      local_mount: /usr/local/etc/slurm
      fs_type: nfs
      mount_options: ""
    - server_ip: cluster-controller-0 # IP or hostname of the slurm controller
      remote_mount: /etc/munge-install
      local_mount: /etc/munge
      fs_type: nfs
      mount_options: ""
    - server_ip: cluster-controller-0 # IP or hostname of the slurm controller
      remote_mount: /apps
      local_mount: /opt/apps
      fs_type: nfs
      mount_options: ""
    - server_ip: cluster-controller-0 # IP or hostname of the slurm controller
      remote_mount: /home/users
      local_mount: /home
      fs_type: nfs
      mount_options: ""
```

[inputns]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md#input\_network\_storage

#### Mount From Another Source

If any of the default mount points exist in another network storage solution,
such as an NFS /home filesystem, a `network_storage` entry can be added similar
to [Create a New Network Storage Entry](#create-a-new-network-storage-entry),
but with a different `server_ip`, as well as other customized options to suit
your needs.

### On Premise Partitions

There are a couple of things to keep in mind when adding the hybrid
configuration when it comes to the existing local partitions in your Slurm
cluster.

First of all, you will now be adding new partitions, the cloud partitions, when
including the configuration produced by the hybrid module. If your current
partitions are setting default values that apply broadly, they will also apply
to the new cloud partitions.

A common issue is when `Nodes=ALL` is set in a local partition definition. This
will create duplicated partitions with the name of the local partition for all
of the cloud partitions. To avoid this, set `Nodes` equal to the actual node
names of the local partition.

If your on premise nodes are using the Slurm [power saving][powersaving]
functionality, see the [Power Saving Operations](#power-saving-operations)
section.

## Creating the Hybrid Configuration with the Cluster Toolkit

With these considerations in mind, you can now move on to creating and
installing the hybrid Cluster Toolkit deployment. To do so, follow the steps in
[deploy-instructions.md].

[deploy-instructions.md]: ./deploy-instructions.md
