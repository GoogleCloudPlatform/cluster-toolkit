# Modules

This directory contains a set of core modules built for the HPC Toolkit. Modules
describe the building blocks of an HPC deployment. The expected fields in a
module are listed in more detail below.

## Available Modules

Modules listed below with the core badge (![core-badge]) are located in this
folder and are tested and maintained by the HPC Toolkit team.

Modules labeled with the community badge (![community-badge]) are contributed by
the community (including the HPC Toolkit team, partners, etc.). Community modules
are located in the [community folder](../community/modules/README.md).

Modules that are still in development and less stable are also labeled with the
experimental badge (![experimental-badge]).

[core-badge]: https://img.shields.io/badge/-core-blue?style=plastic
[community-badge]: https://img.shields.io/badge/-community-%23b8def4?style=plastic
[stable-badge]: https://img.shields.io/badge/-stable-lightgrey?style=plastic
[experimental-badge]: https://img.shields.io/badge/-experimental-%23febfa2?style=plastic

Compute

* **[vm-instance]** ![core-badge] : Creates one or more simple VM instances.
* **[SchedMD-slurm-on-gcp-partition]** ![community-badge] : Creates a partition
  to be used by a [slurm-controller][schedmd-slurm-on-gcp-controller].

[vm-instance]: compute/vm-instance/README.md
[schedmd-slurm-on-gcp-partition]: ../community/modules/compute/SchedMD-slurm-on-gcp-partition/README.md

Database

* **[slurm-cloudsql-federation]** ![community-badge] ![experimental-badge] :
  Creates a [Google SQL Instance](https://cloud.google.com/sql/) meant to be
  integrated with a [slurm-controller][schedmd-slurm-on-gcp-controller].

[slurm-cloudsql-federation]: ../community/modules/database/slurm-cloudsql-federation/README.md

File System

* **[filestore]** ![core-badge] : Creates a [filestore](https://cloud.google.com/filestore) file system.
* **[pre-existing-network-storage]** ![core-badge] : Specifies a
  pre-existing file system to be mounted..
* **[DDN-EXAScaler]** ![community-badge] : Creates
  a [DDN EXAscaler lustre](https://www.ddn.com/partners/google-cloud-platform/)
  file system. This module has
  [license costs](https://console.developers.google.com/marketplace/product/ddnstorage/exascaler-cloud).
* **[Intel-DAOS]** ![community-badge] : Creates
  a [DAOS](https://docs.daos.io/) file system.
* **[nfs-server]** ![community-badge] ![experimental-badge] : Creates a VM and
  configures an NFS server that can be mounted by other VM.

[filestore]: file-system/filestore/README.md
[pre-existing-network-storage]: file-system/pre-existing-network-storage/README.md
[ddn-exascaler]: ../community/modules/file-system/DDN-EXAScaler/README.md
[intel-daos]: ../community/modules/file-system/Intel-DAOS/README.md
[nfs-server]: ../community/modules/file-system/nfs-server/README.md

Monitoring

* **[dashboard]** ![core-badge] : Creates a
  [monitoring dashboard](https://cloud.google.com/monitoring/dashboards) for
  visually tracking a HPC Toolkit deployment.

[dashboard]: monitoring/dashboard/README.md

Network

* **[vpc]** ![core-badge] : Creates a
  [Virtual Private Cloud (VPC)](https://cloud.google.com/vpc) network with
  regional subnetworks and firewall rules.
* **[pre-existing-vpc]** ![core-badge] : Used to connect newly
  built components to a pre-existing VPC network.

[vpc]: network/vpc/README.md
[pre-existing-vpc]: network/pre-existing-vpc/README.md

Packer

* **[custom-image]** ![core-badge] : Creates a custom VM Image
  based on the GCP HPC VM image.

[custom-image]: packer/custom-image/README.md

Project

* **[new-project]** ![community-badge] ![experimental-badge] : Creates a Google Cloud
  Projects.
* **[service-account]** ![community-badge] ![experimental-badge] : Creates [service
  accounts](https://cloud.google.com/iam/docs/service-accounts) for a GCP
  project.
* **[service-enablement]** ![community-badge] ![experimental-badge] : Allows enabling
  various APIs for a Google Cloud Project.

[new-project]: ../community/modules/project/new-project/README.md
[service-account]: ../community/modules/project/service-account/README.md
[service-enablement]: ../community/modules/project/service-enablement/README.md

Scheduler

* **[SchedMD-slurm-on-gcp-controller]** ![community-badge] : Creates a SLURM controller node using
  [slurm-gcp](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/controller)
* **[SchedMD-slurm-on-gcp-login-node]** ![community-badge] : Creates a SLURM login node using
  [slurm-gcp](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/login)

[schedmd-slurm-on-gcp-controller]: ../community/modules/scheduler/SchedMD-slurm-on-gcp-controller/README.md
[schedmd-slurm-on-gcp-login-node]: ../community/modules/scheduler/SchedMD-slurm-on-gcp-login-node/README.md

Scripts

* **[startup-script]** ![core-badge] : Creates a customizable startup script
  that can be fed into compute VMS.
* **[omnia-install]** ![community-badge] ![experimental-badge] : Installs SLURM
  via omnia onto a cluster of compute VMs.
* **[spack-install]** ![community-badge] ![experimental-badge] : Creates a
  startup script to install spack on an instance or the slurm controller.
* **[wait-for-startup]** ![community-badge] ![experimental-badge] : Waits for
  successful completion of a startup script on a compute VM.

[startup-script]: scripts/startup-script/README.md
[omnia-install]: ../community/modules/scripts/omnia-install/README.md
[spack-install]: ../community/modules/scripts/spack-install/README.md
[wait-for-startup]: ../community/modules/scripts/wait-for-startup/README.md

## Module Fields

Find an overview of module fields on the [Google Cloud Docs](https://cloud.google.com/hpc-toolkit/docs/setup/hpc-blueprint).

### Source (Required)

The source is a path or URL that points to the source files for a module. The
actual content of those files is determined by the [kind](#kind-required) of the
module.

A source can be a path which may refer to a module embedded in the `ghpc`
binary or a local file. It can also be a URL pointing to a github path
containing a conforming module.

#### Embedded Modules

Embedded modules are embedded in the ghpc binary during compilation and cannot
be edited. To refer to embedded modules, set the source path to
`modules/<module path>`. The paths match the modules in the repository at
compilation time. For instance, the following code is using the embedded
pre-existing-vpc module.

```yaml
  - source: modules/network/pre-existing-vpc
    kind: terraform
    id: network1
```

#### Local Modules

Local modules point to a module in the file system and can easily be edited.
They are very useful during module development. To use a local module, set
the source to a path starting with `/`, `./`, or `../`. For instance, the
following code is using the local pre-existing-vpc modules.

```yaml
  - source: ./modules/network/pre-existing-vpc
    kind: terraform
    id: network1
```

#### Github Modules

GitHub modules point to a module in GitHub. To use a GitHub module, set
the source to a path starting with `github.com` (over HTTPS) or `git@github.com`
(over SSH). For instance, the following codes are using the GitHub
pre-existing-vpc module.

Get module from GitHub over SSH:

```yaml
  - source: git@github.com:GoogleCloudPlatform/hpc-toolkit.git//modules/network/vpc
    kind: terraform
    id: network1
```

Get module from GitHub over HTTPS:

```yaml
  - source: github.com/GoogleCloudPlatform/hpc-toolkit//modules/network/vpc
    kind: terraform
    id: network1
```

### Kind (Required)

Kind refers to the way in which a module is deployed. Currently, kind can be
either `terraform` or `packer`.

### ID (Required)

The `id` field is used to uniquely identify and reference a defined module.
ID's are used in [variables](../examples/README.md#variables) and become the
name of each module when writing the terraform `main.tf` file. They are also
used in the [use](#use) and [outputs](#outputs) lists described just below.

For terraform modules, the ID will be rendered into the terraform module label
at the top level main.tf file.

### Settings (May Be Required)

The settings field is a map that supplies any user-defined variables for each
module. Settings values can be simple strings, numbers or booleans, but can
also support complex data types like maps and lists of variable depth. These
settings will become the values for the variables defined in either the
`variables.tf` file for Terraform or `variable.pkr.hcl` file for Packer.

For some modules, there are mandatory variables that must be set,
therefore `settings` is a required field in that case. In many situations, a
combination of sensible defaults, deployment variables and used modules can
populated all required settings and therefore the settings field can be left out
entirely.

### Use (Optional)

The `use` field is a powerful way of linking a module to one or more other
modules. When a module "uses" another module, the outputs of the used
module are compared to the settings of the current module. If they have
matching names, and the setting has no explicit value, then it will be set to
the used module's output. For example, see the following blueprint snippet:

```yaml
modules:
- source: modules/network/vpc
  kind: terraform
  id: network1

- resource: modules/compute/vm-instance
  kind: terraform
  id: workstation
  use: [network1]
  settings:
    ...
```

In this snippet, the VM instance `workstation` uses the outputs of vpc
`network1`.

In this case both `network_self_link` and `subnetwork_self_link` in the
[`workstation` settings](compute/vm-instance/README.md#Inputs) will be set
to `$(network1.network_self_link)` and `$(network1.subnetwork_self_link)` which
refer to the [`network1` outputs](network/vpc/README#Outputs)
of the same names.

The order of precedence that `ghpc` uses in determining when to infer a setting
value is the following:

1. Explicitly set in the blueprint by the user
1. Output from a used module, taken in the order provided in the `use` list
1. Deployment variable (`vars`) of the same name
1. Default value for the setting

### Outputs (Optional)

The `outputs` field allows a module-level output to be made available at the
deployment group level and therefore will be available via `terraform output` in
terraform-based deployment groups. This can useful for displaying the IP of a
login node or simply displaying instructions on how to use a module, as we
have in the
[monitoring dashboard module](monitoring/dashboard/README.md#Outputs).

## Common Settings

The following common naming conventions should be used to decrease the verbosity
needed to define a blueprint. This is intentional to allow multiple
modules to share inferred settings from deployment variables. For example, if
all modules are to be created in a single region, that region can be defined as
a deployment variable, which is shared between all moduels without an explicit
setting.

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
creating a new module.

### Terraform Requirements

The module source field must point to a single terraform module. We recommend the
following structure:

* main.tf file composing the terraform modules using provided variables.
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

### Module Role

A module role is a default label applied to modules (ghpc_role), which
conveys what role that module plays within a larger HPC environment.

The modules provided with the HPC toolkit have been divided into roles
matching the names of folders in this directory (ex: compute, file-system etc.).
When possible, custom modules should use these roles so that they match other
modules defined by the toolkit. If a custom module does not fit into these
roles, a new role can be defined.

A module's parent folder will define the module’s role. Therefore,
regardless of where the module is located, the module directory should be
explicitly referenced at least 2 layers deep, where the top layer refers to the
“role” of that module.

If a module is not defined at least 2 layers deep and the ghpc_role label has
not been explicitly set in settings, ghpc_role will default to undefined.

Below we show a few of the modules and their roles (as parent folders).

```text
modules/
├── compute
│   └── vm-instance
├── file-system
│   └── filestore
├── network
│   ├── pre-existing-vpc
│   └── vpc
├── packer
│   └── custom-image
├── scripts
│   ├── omnia-install
│   ├── startup-script
│   └── wait-for-startup
└── third-party
    ├── compute
    ├── file-system
    └── scheduler
```

### Terraform Coding Standards

Any Terraform based modules in the HPC Toolkit repo should implement the following standards:

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
