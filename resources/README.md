# Resources

This directory contains a set of resources built for the HPC Toolkit. Resources
describe the building blocks of an HPC blueprint. The expected fields in a
resource are listed in more detail below.

## Resource Fields

### Source (Required)

The source is a path or URL that points to the source files for a resource. The
actual content of those files is determined by the [kind](#kind-required) of the
resource.

A source can be a path which may refer to a resource embedded in the `ghpc`
binary or a local file. It can also be a URL pointing to a github path
containing a conforming module.

#### Embedded Resources

Embedded resources are embedded in the ghpc binary during compilation and cannot
be edited. To refer to embedded resources, set the source path to
`resources/<resource path>`. The paths match the resources in the repository at
compilation time. For instance, the following code is using the embedded
pre-existing-vpc resource.

```yaml
  - source: resources/network/pre-existing-vpc
    kind: terraform
    id: network1
```

#### Local Resources

Local resources point to a resource in the file system and can easily be edited.
They are very useful during resource development. To use a local resource, set
the source to a path starting with `/`, `./`, or `../`. For instance, the
following code is using the local pre-existing-vpc resource.

```yaml
  - source: ./resources/network/pre-existing-vpc
    kind: terraform
    id: network1
```

#### Github Resources

GitHub resources point to a resource in GitHub. To use a GitHub resource, set
the source to a path starting with `github.com` (over HTTPS) or `git@github.com`
(over SSH). For instance, the following codes are using the GitHub
pre-existing-vpc resource.

Get resource from GitHub over SSH:

```yaml
  - source: git@github.com:GoogleCloudPlatform/hpc-toolkit.git//resources/network/vpc
    kind: terraform
    id: network1
```

Get resource from GitHub over HTTPS:

```yaml
  - source: github.com/GoogleCloudPlatform/hpc-toolkit//resources/network/vpc
    kind: terraform
    id: network1
```

### Kind (Required)

Kind refers to the way in which a resource is deployed. Currently, kind can be
either `terraform` or `packer`.

### ID (Required)

The `id` field is used to uniquely identify and reference a defined resource.
ID's are used in [variables](../examples/README.md#variables) and become the
name of each module when writing terraform resources. They are also used in the
[use](#use) and [outputs](#outputs) lists described just below.

For terraform resources, the ID will be rendered into the terraform module label
at the top level main.tf file.

### Settings (May Be Required)

The settings field is a map that supplies any user-defined variables for each
resource. Settings values can be simple strings, numbers or booleans, but can
also support complex data types like maps and lists of variable depth. These
settings will become the values for the variables defined in either the
`variables.tf` file for Terraform or `variable.pkr.hcl` file for Packer.

For some resources, there are mandatory variables that must be set,
therefore `settings` is a required field in that case. In many situations, a
combination of sensible defaults, global variables and used resources can
populated all required settings and therefore the settings field can be left out
entirely.

### Use (Optional)

The `use` field is a powerful way of linking a resource to one or more other
resources. When a resource "uses" another resource, the outputs of the used
resource are compared to the settings of the current resource. If they have
matching names, and the setting has no explicit value, then it will be set to
the used resource's output. For example, see the following YAML:

```yaml
resources:
- source: resources/network/vpc
  kind: terraform
  id: network1

- resource: resources/compute/simple-instance
  kind: terraform
  id: workstation
  use: [network1]
  settings:
    ...
```

In this snippet, the simple instance, `workstation`, uses the outputs of vpc
`network1`.

In this case both `network_self_link` and `subnetwork_self_link` in the
[`workstation` settings](compute/simple-instance/README.md#Inputs) will be set
to `$(network1.network_self_link)` and `$(network1.subnetwork_self_link)` which
refer to the [`network1` outputs](network/vpc/README#Outputs)
of the same names.

The order of precedence that `ghpc` uses in determining when to infer a setting
value is the following:

1. Explicitly set in the config by the user
1. Output from a used resource, taken in the order provided in the `use` list
1. Global variable (`vars`) of the same name
1. Default value for the setting

### Outputs (Optional)

The `outputs` field allows a resource-level output to be made available at the
resource group level and therefore will be available via `terraform output` in
terraform-based resources groups. This can useful for displaying the IP of a
login node or simply displaying instructions on how to use a resources, as we
have in the
[monitoring dashboard resource](monitoring/dashboard/README.md#Outputs).

## Common Settings

The following common naming conventions should be used to decrease the verbosity
needed to define a blueprint via YAML. This is intentional to allow multiple
resources to share inferred settings from global variables. For example, if all
resources are to be created in a single region, that region can be defined as a
global variable, which is shared between all resources without an explicit
setting.

* **project_id**: The GCP project ID in which to create the resource.
* **deployment_name**: The name of the current deployment of a blueprint. This
  can help to avoid naming conflicts of resources when multiple deployments are
  created from the same set of blueprints.
* **region**: The GCP
  [region](https://cloud.google.com/compute/docs/regions-zones) the resource
  will be created in.
* **zone**: The GCP [zone](https://cloud.google.com/compute/docs/regions-zones)
  the resource will be created in.
* **network_name**: The name of the network a resource will use or connect to.
* **labels**:
  [Labels](https://cloud.google.com/resource-manager/docs/creating-managing-labels)
  added to the resource. In order to include any resource in advanced
  monitoring, labels must be exposed. We strongly recommend that all resources
  expose this variable.

## Writing Custom Resources

Resources are much more flexible by design, however we do define some best practices when creating a new resource.

### Terraform Requirements

The resource source field must point to a single module. We recommend the following structure:

* main.tf file composing the resources using provided variables.
* variables.tf file defining the variables used.
* (Optional) outputs.tf file defining any exported outputs used (if any).
* (Optional) modules directory pointing to submodules needed to create the resource.

### General Best Practices

* Variables for environment-specific values (like project_id) should not be given defaults. This forces the calling module to provide meaningful values.
* Variables should only have zero-value defaults (like null or empty strings) where leaving the variable empty is a valid preference which will not be rejected by the underlying API(s).
* Set good defaults wherever possible. Be opinionated about HPC use cases.
* Follow common variable naming conventions described below

### Resource Role

A resource role is a default label applied to resources (ghpc_role), which
conveys what role that resource plays within a larger HPC environment.

The standard resources provided with the HPC toolkit include 4 roles currently:
compute, file-system, network and scheduler. When possible, custom resources
should use these roles so that they match other resources defined by the
toolkit. If a custom resource does not fit into these roles, a new role can be
defined.

A resource’s parent folder will define the resource’s role. Therefore,
regardless of where the resource is located, the resource directory should be
explicitly referenced 2 layers deep, where the top layer refers to the “role” of
that resource.

If a resource is not defined 2 layers deep and the ghpc_role label has not been
explicitly set in settings, ghpc_role will default to undefined.

Below we show a few of the resources and their roles (as parent folders).

```text
resources/
├── compute
│   └── simple-instance
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

Any Terraform based resources in the HPC Toolkit repo should implement the following standards:

* terraform-docs is used to generate README files for each resource.
* The first parameter listed under a module should be source (when referring to an external implementation).
* The order for parameters in inputs should be:
  * description
  * type
  * default
* The order for parameters in outputs should be:
  * description
  * value

## Available Resources

### Compute

* [**simple-instance**](./compute/simple-instance/README.md): Creates one or
  more simple VM instances.

### Database

*
  [**slurm-cloudsql-federation**](./database/slurm-cloudsql-federation/README.md):
  Creates a [Google SQL Instance](https://cloud.google.com/sql/) meant to be
  integrated with a
  [slurm controller](./third-pary/scheduler/SchedMD-slurm-on-gcp-controller/README.md).

### File System

* [**filestore**](file-system/filestore/README.md): Creates a
  [filestore](https://cloud.google.com/filestore) file system

* [**nfs-server**](file-system/nfs-server/README.md): Creates a VM instance and
  configures an NFS server that can be mounted by other VM instances.

*
  [**pre-existing-network-storage**](file-system/pre-existing-network-storage/README.md):
  Used when specifying a pre-existing file system to be mounted by
  simple_instances and slurm resources.

### Monitoring

* [**dashboard**](monitoring/dashboard/README.md): Creates a
  [monitoring dashboard](https://cloud.google.com/monitoring/dashboards) for
  visually tracking a HPC Toolkit deployment.

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

* [**service-account**](project/service-account/README.md): Creates [service
  accounts](https://cloud.google.com/iam/docs/service-accounts) for a GCP project.

* [**service-enablement**](project/service-enablement/README.md): Allows
  enabling various APIs for a Google Cloud Project

### Scripts

* [**omnia-install**](scripts/omnia-install/README.md): Installs SLURM via omnia
  onto a cluster of compute VMs

* [**spack-install**](scripts/spack-install/README.md): Creates a startup script
  to install spack on an instance or the slurm controller

* [**startup-script**](scripts/startup-script/README.md): Creates a customizable
  startup script that can be fed into compute VMS

* [**wait-for-startup**](scripts/wait-for-startup/README.md): Waits for
  successful completion of a startup script on a compute VM

### Third Party

#### Compute (third party)

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

#### File System (third party)

* [**DDN-EXAScaler**](third-party/file-system/DDN-EXAScaler/README.md): Creates
  a DDN Exascaler lustre](<https://www.ddn.com/partners/google-cloud-platform/>)
  file system. This resource has
  [license costs](https://console.developers.google.com/marketplace/product/ddnstorage/exascaler-cloud).
