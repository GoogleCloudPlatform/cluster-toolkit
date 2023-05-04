# Modules

This directory contains a set of core modules built for the HPC Toolkit. Modules
describe the building blocks of an HPC deployment. The expected fields in a
module are listed in more detail [below](#module-fields). Blueprints can be
extended in functionality by incorporating [modules from GitHub
repositories][ghmods].

[ghmods]: #github-modules

## All Modules

Modules from various sources are all listed here for visibility. Badges are used
to indicate the source and status of many of these resources.

Modules listed below with the ![core-badge] badge are located in this
folder and are tested and maintained by the HPC Toolkit team.

Modules labeled with the ![community-badge] badge are contributed by
the community (including the HPC Toolkit team, partners, etc.). Community modules
are located in the [community folder](../community/modules/README.md).

Modules that are still in development and less stable are labeled with the
![experimental-badge] badge.

[core-badge]: https://img.shields.io/badge/-core-blue?style=plastic
[community-badge]: https://img.shields.io/badge/-community-%23b8def4?style=plastic
[stable-badge]: https://img.shields.io/badge/-stable-lightgrey?style=plastic
[experimental-badge]: https://img.shields.io/badge/-experimental-%23febfa2?style=plastic

### Compute

* **[vm-instance]** ![core-badge] : Creates one or more VM instances.
* **[SchedMD-slurm-on-gcp-partition]** ![community-badge] : Creates a partition
  to be used by a [slurm-controller][schedmd-slurm-on-gcp-controller].
* **[schedmd-slurm-gcp-v5-partition]** ![community-badge] ![experimental-badge] :
  Creates a partition to be used by a [slurm-controller][schedmd-slurm-gcp-v5-controller].
* **[schedmd-slurm-gcp-v5-node-group]** ![community-badge] ![experimental-badge] :
  Creates a node group to be used by the [schedmd-slurm-gcp-v5-partition] module.
* **[htcondor-execute-point]** ![community-badge] ![experimental-badge] :
  Manages a group of execute points for use in an [HTCondor
  pool][htcondor-configure].
* **[pbspro-execution]** ![community-badge] ![experimental-badge] :
  Creates execution hosts for use in a PBS Professional cluster.

[vm-instance]: compute/vm-instance/README.md
[schedmd-slurm-on-gcp-partition]: ../community/modules/compute/SchedMD-slurm-on-gcp-partition/README.md
[schedmd-slurm-gcp-v5-partition]: ../community/modules/compute/schedmd-slurm-gcp-v5-partition/README.md
[schedmd-slurm-gcp-v5-node-group]: ../community/modules/compute/schedmd-slurm-gcp-v5-node-group/README.md
[htcondor-execute-point]: ../community/modules/compute/htcondor-execute-point/README.md
[pbspro-execution]: ../community/modules/compute/pbspro-execution/README.md

### Database

* **[slurm-cloudsql-federation]** ![community-badge] ![experimental-badge] :
  Creates a [Google SQL Instance](https://cloud.google.com/sql/) meant to be
  integrated with a [slurm-controller][schedmd-slurm-on-gcp-controller].

[slurm-cloudsql-federation]: ../community/modules/database/slurm-cloudsql-federation/README.md

### File System

* **[filestore]** ![core-badge] : Creates a [filestore](https://cloud.google.com/filestore) file system.
* **[pre-existing-network-storage]** ![core-badge] : Specifies a
  pre-existing file system that can be mounted on a VM.
* **[DDN-EXAScaler]** ![community-badge] : Creates
  a [DDN EXAscaler lustre](https://www.ddn.com/partners/google-cloud-platform/)
  file system. This module has
  [license costs](https://console.developers.google.com/marketplace/product/ddnstorage/exascaler-cloud).
* **[Intel-DAOS]** ![community-badge] : Creates
  a [DAOS](https://docs.daos.io/) file system.
* **[cloud-storage-bucket]** ![community-badge] ![experimental-badge] : Creates a Google Cloud Storage (GCS) bucket.
* **[nfs-server]** ![community-badge] ![experimental-badge] : Creates a VM and
  configures an NFS server that can be mounted by other VM.

[filestore]: file-system/filestore/README.md
[pre-existing-network-storage]: file-system/pre-existing-network-storage/README.md
[ddn-exascaler]: ../community/modules/file-system/DDN-EXAScaler/README.md
[intel-daos]: ../community/modules/file-system/Intel-DAOS/README.md
[nfs-server]: ../community/modules/file-system/nfs-server/README.md
[cloud-storage-bucket]: ../community/modules/file-system/cloud-storage-bucket/README.md

### Monitoring

* **[dashboard]** ![core-badge] : Creates a
  [monitoring dashboard](https://cloud.google.com/monitoring/dashboards) for
  visually tracking a HPC Toolkit deployment.

[dashboard]: monitoring/dashboard/README.md

### Network

* **[vpc]** ![core-badge] : Creates a
  [Virtual Private Cloud (VPC)](https://cloud.google.com/vpc) network with
  regional subnetworks and firewall rules.
* **[pre-existing-vpc]** ![core-badge] : Used to connect newly
  built components to a pre-existing VPC network.

[vpc]: network/vpc/README.md
[pre-existing-vpc]: network/pre-existing-vpc/README.md

### Packer

* **[custom-image]** ![core-badge] : Creates a custom VM Image
  based on the GCP HPC VM image.

[custom-image]: packer/custom-image/README.md

### Project

* **[new-project]** ![community-badge] ![experimental-badge] : Creates a Google
  Cloud Project.
* **[service-account]** ![community-badge] ![experimental-badge] : Creates [service
  accounts](https://cloud.google.com/iam/docs/service-accounts) for a GCP
  project.
* **[service-enablement]** ![community-badge] ![experimental-badge] : Allows enabling
  various APIs for a Google Cloud Project.

[new-project]: ../community/modules/project/new-project/README.md
[service-account]: ../community/modules/project/service-account/README.md
[service-enablement]: ../community/modules/project/service-enablement/README.md

### Remote Desktop

* **[chrome-remote-desktop]** ![community-badge] ![experimental-badge] : Creates
  a GPU accelerated Chrome Remote Desktop.

[chrome-remote-desktop]: ../community/modules/remote-desktop/chrome-remote-desktop/README.md

### Scheduler

* **[batch-job-template]** ![core-badge] : Creates a Google Cloud Batch job
  template that works with other Toolkit modules.
* **[batch-login-node]** ![core-badge] : Creates a VM that can be used for
  submission of Google Cloud Batch jobs.
* **[gke-cluster]** ![community-badge] ![experimental-badge] : Creates a
  Kubernetes cluster using GKE.
* **[schedmd-slurm-gcp-v5-controller]** ![community-badge] ![experimental-badge] :
  Creates a Slurm controller node using [slurm-gcp-version-5].
* **[schedmd-slurm-gcp-v5-login]** ![community-badge] ![experimental-badge] :
  Creates a Slurm login node using [slurm-gcp-version-5].
* **[schedmd-slurm-gcp-v5-hybrid]** ![community-badge] ![experimental-badge] :
  Creates hybrid Slurm partition configuration files using [slurm-gcp-version-5].
* **[SchedMD-slurm-on-gcp-controller]** ![community-badge] : Creates a Slurm
  controller node using [slurm-gcp].
* **[SchedMD-slurm-on-gcp-login-node]** ![community-badge] : Creates a Slurm
  login node using [slurm-gcp].
* **[htcondor-configure]** ![community-badge] ![experimental-badge] : Creates
  Toolkit runners and service accounts to configure an HTCondor pool.
* **[pbspro-client]** ![community-badge] ![experimental-badge] : Creates
  a client host for submitting jobs to a PBS Professional cluster.
* **[pbspro-server]** ![community-badge] ![experimental-badge] : Creates
  a server host for operating a PBS Professional cluster.

[batch-job-template]: ../modules/scheduler/batch-job-template/README.md
[batch-login-node]: ../modules/scheduler/batch-login-node/README.md
[gke-cluster]: ../community/modules/scheduler/gke-cluster/README.md
[htcondor-configure]: ../community/modules/scheduler/htcondor-configure/README.md
[schedmd-slurm-gcp-v5-controller]: ../community/modules/scheduler/schedmd-slurm-gcp-v5-controller/README.md
[schedmd-slurm-gcp-v5-login]: ../community/modules/scheduler/schedmd-slurm-gcp-v5-login/README.md
[schedmd-slurm-gcp-v5-hybrid]: ../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md
[schedmd-slurm-on-gcp-controller]: ../community/modules/scheduler/SchedMD-slurm-on-gcp-controller/README.md
[schedmd-slurm-on-gcp-login-node]: ../community/modules/scheduler/SchedMD-slurm-on-gcp-login-node/README.md
[slurm-gcp]: https://github.com/SchedMD/slurm-gcp/tree/v4.2.1
[slurm-gcp-version-5]: https://github.com/SchedMD/slurm-gcp/tree/5.2.0
[pbspro-client]: ../community/modules/scheduler/pbspro-client/README.md
[pbspro-server]: ../community/modules/scheduler/pbspro-server/README.md

### Scripts

* **[startup-script]** ![core-badge] : Creates a customizable startup script
  that can be fed into compute VMs.
* **[htcondor-install]** ![community-badge] ![experimental-badge] : Creates
  a startup script to install HTCondor and exports a list of required APIs
* **[omnia-install]** ![community-badge] ![experimental-badge] : Installs Slurm
  via [Dell Omnia](https://github.com/dellhpc/omnia) onto a cluster of VMs
  instances.
* **[pbspro-preinstall]** ![community-badge] ![experimental-badge] : Creates a
  Cloud Storage bucket in which to save PBS Professional RPM packages for use
  by PBS clusters.
* **[pbspro-install]** ![community-badge] ![experimental-badge] : Creates a
  Toolkit runner to install [PBS Professional][pbspro] from RPM packages.
* **[pbspro-qmgr]** ![community-badge] ![experimental-badge] : Creates a
  Toolkit runner to run common `qmgr` commands when configuring a PBS
  Professional cluster.
* **[spack-install]** ![community-badge] ![experimental-badge] : Creates a
  startup script to install [Spack](https://github.com/spack/spack) on an
  instance or a slurm login or controller.
* **[wait-for-startup]** ![community-badge] ![experimental-badge] : Waits for
  successful completion of a startup script on a compute VM.

[startup-script]: scripts/startup-script/README.md
[htcondor-install]: ../community/modules/scripts/htcondor-install/README.md
[omnia-install]: ../community/modules/scripts/omnia-install/README.md
[spack-install]: ../community/modules/scripts/spack-install/README.md
[wait-for-startup]: ../community/modules/scripts/wait-for-startup/README.md
[pbspro-install]: ../community/modules/scripts/pbspro-install/README.md
[pbspro-preinstall]: ../community/modules/scripts/pbspro-preinstall/README.md
[pbspro-qmgr]: ../community/modules/scripts/pbspro-qmgr/README.md
[pbspro]: https://www.altair.com/pbs-professional

## Module Fields

### ID (Required)

The `id` field is used to uniquely identify and reference a defined module.
ID's are used in [variables](../examples/README.md#variables) and become the
name of each module when writing the terraform `main.tf` file. They are also
used in the [use](#use-optional) and [outputs](#outputs-optional) lists
described below.

For terraform modules, the ID will be rendered into the terraform module label
at the top level main.tf file.

### Source (Required)

The source is a path or URL that points to the source files for a module. The
actual content of those files is determined by the [kind](#kind-may-be-required) of the
module.

A source can be a path which may refer to a module embedded in the `ghpc`
binary or a local file. It can also be a URL pointing to a GitHub path
containing a conforming module.

#### Embedded Modules

Embedded modules are embedded in the ghpc binary during compilation and cannot
be edited. To refer to embedded modules, set the source path to
`modules/<<MODULE_PATH>>`.

The paths match the modules in the repository at compilation time. You can
review the directory structure of [the core modules](./) and
[community modules](../community/modules/) to determine which path to use. For
example, the following code is using the embedded pre-existing-vpc module:

```yaml
  - id: network1
    source: modules/network/pre-existing-vpc
```

#### Local Modules

Local modules point to a module in the file system and can easily be edited.
They are very useful during module development. To use a local module, set
the source to a path starting with `/`, `./`, or `../`. For instance, the
following module definition refers the local pre-existing-vpc modules.

```yaml
  - id: network1
    source: ./modules/network/pre-existing-vpc
```

> **_NOTE:_** This example would have to be run from the HPC Toolkit repository
> directory, otherwise the path would need to be updated to point at the correct
> directory.

#### GitHub Modules

To use a Terraform module available on GitHub, set the source to a path starting
with `github.com` (over HTTPS) or `git@github.com` (over SSH). For instance, the
following module definitions are sourcing the vpc module by pointing at the HPC
Toolkit GitHub repository:

Get module from GitHub over SSH:

```yaml
  - id: network1
    source: git@github.com:GoogleCloudPlatform/hpc-toolkit.git//modules/network/vpc
```

Get module from GitHub over HTTPS:

```yaml
  - id: network1
    source: github.com/GoogleCloudPlatform/hpc-toolkit//modules/network/vpc
```

Both examples above use the [double-slash notation][tfsubdir] (`//`) to indicate
the root directory of the git repository and the remainder of the path indicates
the location of the Terraform module.

Additionally, [specific revisions of a remote module][tfrev] can be selected by
any valid [git reference][gitref]. Typically, these are a git branch, commit
hash or tag. The [Intel DAOS blueprint][daos-cluster.yaml] makes extensive use
of this feature. For example, to temporarily point to a development copy of the
Toolkit vpc module, use:

```yaml
  - id: network1
    source: github.com/GoogleCloudPlatform/hpc-toolkit//modules/network/vpc?ref=develop
```

[tfrev]: https://www.terraform.io/language/modules/sources#selecting-a-revision
[gitref]: https://git-scm.com/book/en/v2/Git-Tools-Revision-Selection#_single_revisions
[tfsubdir]: https://www.terraform.io/language/modules/sources#modules-in-package-sub-directories
[daos-cluster.yaml]: ../community/examples/intel/daos-cluster.yaml

#### Generic Git Modules
To use a Terraform module available in a non-GitHub git repository such as
gitlab, set the source to a path starting `git::`. Two Standard git protocols
are supported, `git::https://` for HTTPS or `git::git@github.com` for SSH.

Additional formatting and features after `git::` are identical to that of the
[GitHub Modules](#github-modules) described above.

### Kind (May be Required)

`kind` refers to the way in which a module is deployed. Currently, `kind` can be
either `terraform` or `packer`. It must be specified for modules of type
`packer`. If omitted, it will default to `terraform`.

### Settings (May Be Required)

The settings field is a map that supplies any user-defined variables for each
module. Settings values can be simple strings, numbers or booleans, but can
also support complex data types like maps and lists of variable depth. These
settings will become the values for the variables defined in either the
`variables.tf` file for Terraform or `variable.pkr.hcl` file for Packer.

For some modules, there are mandatory variables that must be set,
therefore `settings` is a required field in that case. In many situations, a
combination of sensible defaults, deployment variables and used modules can
populated all required settings and therefore the settings field can be omitted.

### Use (Optional)

The `use` field is a powerful way of linking a module to one or more other
modules. When a module "uses" another module, the outputs of the used
module are compared to the settings of the current module. If they have
matching names and the setting has no explicit value, then it will be set to
the used module's output. For example, see the following blueprint snippet:

```yaml
modules:
- id: network1
  source: modules/network/vpc

- id: workstation
  source: modules/compute/vm-instance
  use: [network1]
  settings:
  ...
```

In this snippet, the VM instance `workstation` uses the outputs of vpc
`network1`.

In this case both `network_self_link` and `subnetwork_self_link` in the
[workstation settings](compute/vm-instance/README.md#Inputs) will be set
to `$(network1.network_self_link)` and `$(network1.subnetwork_self_link)` which
refer to the [network1 outputs](network/vpc/README#Outputs)
of the same names.

The order of precedence that `ghpc` uses in determining when to infer a setting
value is in the following priority order:

1. Explicitly set in the blueprint using the `settings` field
1. Output from a used module, taken in the order provided in the `use` list
1. Deployment variable (`vars`) of the same name
1. Default value for the setting

> **_NOTE:_** See the
> [network storage documentation](./../docs/network_storage.md) for more
> information about mounting network storage file systems via the `use` field.

### Outputs (Optional)

The `outputs` field adds the output of individual Terraform modules to the
output of its deployment group. This enables the value to be available via
`terraform output`. This can useful for displaying the IP of a login node or
priting instructions on how to use a module, as we have in the
[monitoring dashboard module](monitoring/dashboard/README.md#Outputs).

The outputs field is a lists that it can be in either of two formats: a string
equal to the name of the module output, or a map specifying the `name`,
`description`, and whether the value is `sensitive` and should be suppressed
from the standard output of Terraform commands. An example is shown below
that displays the internal and public IP addresses of a VM created by the
vm-instance module:

```yaml
  - id: vm
    source: modules/compute/vm-instance
    use:
    - network1
    settings:
      machine_type: e2-medium
    outputs:
    - internal_ip
    - name: external_ip
      description: "External IP of VM"
      sensitive: true
```

The outputs shown after running Terraform apply will resemble:

```text
Apply complete! Resources: 7 added, 0 changed, 0 destroyed.

Outputs:

external_ip_simplevm = <sensitive>
internal_ip_simplevm = [
  "10.128.0.19",
]
```

### Required Services (APIs) (optional)

Each Toolkit module depends upon Google Cloud services ("APIs") being enabled
in the project used by the HPC environment. For example, the [creation of
VMs](compute/vm-instance/) requires the Compute Engine API
(compute.googleapis.com). The [startup-script](scripts/startup-script/) module
requires the Cloud Storage API (storage.googleapis.com) for storage of the
scripts themselves. Each module includes in the Toolkit source code describes
its required APIs internally. The Toolkit will merge the requiements from all
modules and [automatically validate](../README.md#blueprint-validation) that all
APIs are enabled in the project specified by `$(vars.project_id)`.

For advanced multi-project use cases and for modules not included with the
Toolkit, you may manually add required APIs to each module with the following
format:

```yaml
deployment_groups:
- group: primary
  modules:
  ...
  - id: examplevm
    source: modules/example/module
    required_apis:
      $(vars.project_id):
      - compute.googleapis.com
      - storage.googleapis.com
      $(vars.other_project_id):
      - storage.googleapis.com
      explicit-project-id:
      - file.googleapis.com
    settings:
    ...
```

## Common Settings

The following common naming conventions should be used to decrease the verbosity
needed to define a blueprint. This is intentional to allow multiple
modules to share inferred settings from deployment variables or from other
modules listed under the `use` field.

For example, if all modules are to be created in a single region, that region
can be defined as a deployment variable named `region`, which is shared between
all modules without an explicit setting. Similarly, if many modules need to be
connected to the same VPC network, they all can add the vpc module ID to their
`use` list so that `network_name` would be inferred from that vpc module rather
than having to set it manually.

* **project_id**: The GCP project ID in which to create the GCP resources.
* **deployment_name**: The name of the current deployment of a blueprint. This
  can help to avoid naming conflicts of modules when multiple deployments are
  created from the same blueprint.
* **region**: The GCP
  [region](https://cloud.google.com/compute/docs/regions-zones) the module
  will be created in.
* **zone**: The GCP [zone](https://cloud.google.com/compute/docs/regions-zones)
  the module will be created in.
* **network_name**: The name of the network a module will use or connect to.
* **labels**:
  [Labels](https://cloud.google.com/resource-manager/docs/creating-managing-labels)
  added to the module. In order to include any module in advanced
  monitoring, labels must be exposed. We strongly recommend that all modules
  expose this variable.

## Writing Custom HPC Modules

Modules are flexible by design, however we do define some best practices when
creating a new module meant to be used with the HPC Toolkit.

### Terraform Requirements

The module source field must point to a single terraform module. We recommend
the following structure:

* main.tf file composing the terraform resources using provided variables.
* variables.tf file defining the variables used.
* (Optional) outputs.tf file defining any exported outputs used (if any).
* (Optional) modules/ sub-directory pointing to submodules needed to create the
  top level module.

### General Best Practices

* Variables for environment-specific values (like project_id) should not be
  given defaults. This forces the calling module to provide meaningful values.
* Variables should only have zero-value defaults (like null or empty strings)
  where leaving the variable empty is a valid preference which will not be
  rejected by the underlying API(s).
* Set good defaults wherever possible. Be opinionated about HPC use cases.
* Follow common variable [naming conventions](#common-settings).

### Terraform Coding Standards

Any Terraform based modules in the HPC Toolkit should implement the following
standards:

* terraform-docs is used to generate README files for each module.
* The first parameter listed under a module should be source (when referring to
  an external implementation).
* The order for parameters in inputs should be:
  * description
  * type
  * default
* The order for parameters in outputs should be:
  * description
  * value
