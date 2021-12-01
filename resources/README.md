# Resources

This directory contains a set of resources built for the HPC Toolkit. These
resources can be used to define components of an HPC cluster.

## Referring to resources

There are two ways of referring to resources from a configuration YAML: using an embedded resource or a local resource.

Embedded resources are embedded in the ghpc binary during compilation and cannot be edited. To refer to embedded resources, set the source path to `resources/<resource path>`. The paths match the resources in the repository at compilation time. For instance, the following code is using the embedded pre-existing-vpc resource.

```
  - source: resources/network/pre-existing-vpc
    kind: terraform
    id: network1
```

Local resources point to a resource in the file system and can easily be edited. They are very useful during resource development. To use a local resource, set the source to a path starting with `/`, `./`, or `../`. For instance, the following code is using the local pre-existing-vpc resource.

```
  - source: ./resources/network/pre-existing-vpc
    kind: terraform
    id: network1
```

## Common Settings
There are a few common setting names that are consistent accross different
HPC Toolkit resources. This is intentional to allow multiple resources to share
inferred settings from global variables. These variables are listed and
described below.

* **project_id**: The associated GCP project ID of the project a resource (or
resources) will be created.
* **deployment_name**: The name of the current deployment of a blueprint. This
can be changed either in the blueprint itself as needed or in the input yaml.
* **region**: The GCP [region](https://cloud.google.com/compute/docs/regions-zones)
for the resource(s)
* **zone**: The GCP [zone](https://cloud.google.com/compute/docs/regions-zones)
for the resource(s)
* **network_name**: The name of the network a resource will use or connect to.

## List

### Compute

* [**simple-instance**](./compute/simple-instance/README.md): Creates one or
more simple VM instances.

### File System

* [**filestore**](file-system/filestore/README.md): Creates a
[filestore](https://cloud.google.com/filestore) file system

* [**pre-existing-network-storage**](file-system/pre-existing-network-storage/README.md):
Used when specifying a pre-existing file system to be mounted by
simple_instances and slurm resources.


### Network

* [**vpc**](network/vpc/README.md): Creates a
[Virtual Private Cloud (VPC)](https://cloud.google.com/vpc) network with
regional subnetworks and firewall rules.

* [**pre-existing-vpc**](network/pre-existing-vpc/README.md): Connects to a
pre-existing VPC network. Useful for connecting newly built components to an
existing network.

### Packer

* [**custom-image**](packer/custom-image/README.md): Creates a custom VM Image
based on the GCP HPC VM image

### Project

* [**new-project**](project/new-project/README.md): Creates a Google Cloud Projects

* [**service-enablement**](project/service-enablement/README.md): Allows enabling various APIs for a Google Cloud Project

### Scripts

* [**omnia-install**](scripts/omnia-install/README.md): Installs SLURM via omnia onto a cluster of compute VMs

* [**startup-script**](scripts/startup-script/README.md): Creates a customizable
startup script that can be fed into compute VMS

* [**wait-for-startup**](scripts/wait-for-startup/README.md): Waits for
successful completion of a startup script on a compute VM

### Third Party

#### Compute

* [**SchedMD-slurm-on-gcp-partition**](third-party/compute/SchedMD-slurm-on-gcp-partition/README.md):
Creates a SLURM partition that can be used by the
SchedMD-slurm_on_gcp_controller.

#### Scheduler

* [**SchedMD-slurm-on-gcp-controller**](third-party/scheduler/SchedMD-slurm-on-gcp-controller/README.md):
Creates a SLURM controller node using
[slurm-gcp](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/controller)

* [**SchedMD-slurm-on-gcp-login-node**](third-party/scheduler/SchedMD-slurm-on-gcp-login-node/README.md):
Creates a SLURM login node using
[slurm-gcp](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/login)

#### File System

* [**DDN-EXAScaler**](third-party/file-system/DDN-EXAScaler/README.md): Creates a
[DDN Exascaler lustre](https://www.ddn.com/partners/google-cloud-platform/) file system. This resource has [license costs](https://pantheon.corp.google.com/marketplace/product/ddnstorage/exascaler-cloud).
