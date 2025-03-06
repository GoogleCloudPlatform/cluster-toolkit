# Modules

This directory contains a set of core modules built for the Cluster Toolkit. Modules
describe the building blocks of an AI/ML and HPC deployment. The expected fields in a
module are listed in more detail [below](#module-fields). Blueprints can be
extended in functionality by incorporating [modules from GitHub
repositories][ghmods].

[ghmods]: #github-modules

## All Modules

Modules from various sources are all listed here for visibility. Badges are used
to indicate the source and status of many of these resources.

Modules listed below with the ![core-badge] badge are located in this
folder and are tested and maintained by the Cluster Toolkit team.

Modules labeled with the ![community-badge] badge are contributed by
the community (including the Cluster Toolkit team, partners, etc.). Community modules
are located in the [community folder](../community/modules/README.md).

Modules labeled with the ![deprecated-badge] badge are now deprecated and may be
removed in the future. Customers are advised to transition to alternatives.

Modules that are still in development and less stable are labeled with the
![experimental-badge] badge.

[core-badge]: https://img.shields.io/badge/-core-blue?style=plastic
[community-badge]: https://img.shields.io/badge/-community-%23b8def4?style=plastic
[stable-badge]: https://img.shields.io/badge/-stable-lightgrey?style=plastic
[experimental-badge]: https://img.shields.io/badge/-experimental-%23febfa2?style=plastic
[deprecated-badge]: https://img.shields.io/badge/-deprecated-%23fea2a2?style=plastic

### Compute

* **[vm-instance]** ![core-badge] : Creates one or more VM instances.
* **[schedmd-slurm-gcp-v6-partition]** ![core-badge] :
  Creates a partition to be used by a [slurm-controller][schedmd-slurm-gcp-v6-controller].
* **[schedmd-slurm-gcp-v6-nodeset]** ![core-badge] :
  Creates a nodeset to be used by the [schedmd-slurm-gcp-v6-partition] module.
* **[schedmd-slurm-gcp-v6-nodeset-tpu]** ![core-badge] :
  Creates a TPU nodeset to be used by the [schedmd-slurm-gcp-v6-partition] module.
* **[schedmd-slurm-gcp-v6-nodeset-dynamic]** ![core-badge] ![experimental-badge]:
  Creates a dynamic nodeset to be used by the [schedmd-slurm-gcp-v6-partition] module and instance template.
* **[gke-node-pool]** ![core-badge] ![experimental-badge] : Creates a
  Kubernetes node pool using GKE.
* **[resource-policy]** ![core-badge] ![experimental-badge] : Create a resource policy for compute engines that can be applied to gke-node-pool's nodes.
* **[gke-job-template]** ![core-badge] ![experimental-badge] : Creates a
  Kubernetes job file to be used with a [gke-node-pool].
* **[htcondor-execute-point]** ![community-badge] ![experimental-badge] :
  Manages a group of execute points for use in an [HTCondor
  pool][htcondor-setup].
* **[pbspro-execution]** ![community-badge] ![experimental-badge] :
  Creates execution hosts for use in a PBS Professional cluster.
* **[mig]** ![community-badge] ![experimental-badge] : Creates a Managed Instance Group.
* **[notebook]** ![community-badge] ![experimental-badge] : Creates a Vertex AI
  Notebook. Primarily used for [FSI - MonteCarlo Tutorial][fsi-montecarlo-on-batch-tutorial].

[vm-instance]: compute/vm-instance/README.md
[gke-node-pool]: ../modules/compute/gke-node-pool/README.md
[resource-policy]: ../modules/compute/resource-policy/README.md
[gke-job-template]: ../modules/compute/gke-job-template/README.md
[schedmd-slurm-gcp-v6-partition]: ../community/modules/compute/schedmd-slurm-gcp-v6-partition/README.md
[schedmd-slurm-gcp-v6-nodeset]: ../community/modules/compute/schedmd-slurm-gcp-v6-nodeset/README.md
[schedmd-slurm-gcp-v6-nodeset-tpu]: ../community/modules/compute/schedmd-slurm-gcp-v6-nodeset-tpu/README.md
[schedmd-slurm-gcp-v6-nodeset-dynamic]: ../community/modules/compute/schedmd-slurm-gcp-v6-nodeset-dynamic/README.md
[htcondor-execute-point]: ../community/modules/compute/htcondor-execute-point/README.md
[pbspro-execution]: ../community/modules/compute/pbspro-execution/README.md
[mig]: ../community/modules/compute/mig/README.md
[notebook]: ../community/modules/compute/notebook/README.md
[fsi-montecarlo-on-batch-tutorial]: ../docs/tutorials/fsi-montecarlo-on-batch/README.md

### Database

* **[slurm-cloudsql-federation]** ![community-badge] ![experimental-badge] :
  Creates a [Google SQL Instance](https://cloud.google.com/sql/) meant to be
  integrated with a [slurm-controller][schedmd-slurm-gcp-v6-controller].
* **[bigquery-dataset]** ![community-badge] ![experimental-badge] : Creates a BQ
  dataset. Primarily used for [FSI - MonteCarlo Tutorial][fsi-montecarlo-on-batch-tutorial].
* **[bigquery-table]** ![community-badge] ![experimental-badge] : Creates a BQ
  table. Primarily used for
  [FSI - MonteCarlo Tutorial][fsi-montecarlo-on-batch-tutorial].

[slurm-cloudsql-federation]: ../community/modules/database/slurm-cloudsql-federation/README.md
[bigquery-dataset]: ../community/modules/database/bigquery-dataset/README.md
[bigquery-table]: ../community/modules/database/bigquery-table/README.md
[fsi-montecarlo-on-batch]: ../community/modules/files/fsi-montecarlo-on-batch/README.md

### File System

* **[filestore]** ![core-badge] : Creates a [filestore](https://cloud.google.com/filestore) file system.
* **[parallelstore]** ![core-badge] ![experimental-badge]: Creates a [parallelstore](https://cloud.google.com/parallelstore) file system.
* **[pre-existing-network-storage]** ![core-badge] : Specifies a
  pre-existing file system that can be mounted on a VM.
* **[DDN-EXAScaler]** ![community-badge] : Creates
  a [DDN EXAscaler lustre](https://www.ddn.com/partners/google-cloud-platform/)
  file system. This module has
  [license costs](https://console.developers.google.com/marketplace/product/ddnstorage/exascaler-cloud).
* **[cloud-storage-bucket]** ![community-badge] ![experimental-badge] : Creates a Google Cloud Storage (GCS) bucket.
* **[gke-persistent-volume]** ![core-badge] ![experimental-badge] : Creates persistent volumes and persistent volume claims for shared storage.
* **[nfs-server]** ![community-badge] ![experimental-badge] : Creates a VM and
  configures an NFS server that can be mounted by other VM.

[filestore]: file-system/filestore/README.md
[parallelstore]: file-system/parallelstore/README.md
[pre-existing-network-storage]: file-system/pre-existing-network-storage/README.md
[ddn-exascaler]: ../community/modules/file-system/DDN-EXAScaler/README.md
[nfs-server]: ../community/modules/file-system/nfs-server/README.md
[cloud-storage-bucket]: ../community/modules/file-system/cloud-storage-bucket/README.md
[gke-persistent-volume]: ../modules/file-system/gke-persistent-volume/README.md

### Monitoring

* **[dashboard]** ![core-badge] : Creates a
  [monitoring dashboard](https://cloud.google.com/monitoring/dashboards) for
  visually tracking a Cluster Toolkit deployment.

[dashboard]: monitoring/dashboard/README.md

### Network

* **[vpc]** ![core-badge] : Creates a
  [Virtual Private Cloud (VPC)](https://cloud.google.com/vpc) network with
  regional subnetworks and firewall rules.
* **[multivpc]** ![core-badge] ![experimental-badge]: Creates a variable
  number of VPC networks using the [vpc] module.
* **[pre-existing-vpc]** ![core-badge] : Used to connect newly
  built components to a pre-existing VPC network.
* **[firewall-rules]** ![core-badge] ![experimental-badge] : Add custom firewall
  rules to existing networks (commonly used with [pre-existing-vpc]).
* **[private-service-access]** ![community-badge] ![experimental-badge] :
  Configures Private Services Access for a VPC network (commonly used with [filestore] and [slurm-cloudsql-federation]).

[vpc]: network/vpc/README.md
[multivpc]: network/multivpc/README.md
[pre-existing-vpc]: network/pre-existing-vpc/README.md
[firewall-rules]: network/firewall-rules/README.md
[private-service-access]: ../community/modules/network/private-service-access/README.md

### Packer

* **[custom-image]** ![core-badge] : Creates a custom VM Image
  based on the GCP HPC VM image.

[custom-image]: packer/custom-image/README.md

### Project

* **[service-account]** ![community-badge] ![experimental-badge] : Creates [service
  accounts](https://cloud.google.com/iam/docs/service-accounts) for a GCP
  project.
* **[service-enablement]** ![community-badge] ![experimental-badge] : Allows enabling
  various APIs for a Google Cloud Project.

[service-account]: ../community/modules/project/service-account/README.md
[service-enablement]: ../community/modules/project/service-enablement/README.md

### Pub/Sub

* **[topic]** ![community-badge] ![experimental-badge] : Creates a
Pub/Sub topic. Primarily used for [FSI - MonteCarlo Tutorial][fsi-montecarlo-on-batch-tutorial].
* **[bigquery-sub]** ![community-badge] ![experimental-badge] : Creates a
Pub/Sub subscription. Primarily used for [FSI - MonteCarlo Tutorial][fsi-montecarlo-on-batch-tutorial].

[topic]: ../community/modules/pubsub/topic/README.md
[bigquery-sub]: ../community/modules/pubsub/bigquery-sub/README.md

### Remote Desktop

* **[chrome-remote-desktop]** ![community-badge] ![experimental-badge] : Creates
  a GPU accelerated Chrome Remote Desktop.

[chrome-remote-desktop]: ../community/modules/remote-desktop/chrome-remote-desktop/README.md

### Scheduler

* **[batch-job-template]** ![core-badge] : Creates a Google Cloud Batch job
  template that works with other Toolkit modules.
* **[batch-login-node]** ![core-badge] : Creates a VM that can be used for
  submission of Google Cloud Batch jobs.
* **[gke-cluster]** ![core-badge] ![experimental-badge] : Creates a
  Kubernetes cluster using GKE.
* **[pre-existing-gke-cluster]** ![core-badge] ![experimental-badge] : Retrieves an existing GKE cluster. Substitute for ([gke-cluster]) module.
* **[schedmd-slurm-gcp-v6-controller]** ![core-badge] :
  Creates a Slurm controller node using [slurm-gcp-version-6].
* **[schedmd-slurm-gcp-v6-login]** ![core-badge] :
  Creates a Slurm login node using [slurm-gcp-version-6].
* **[htcondor-setup]** ![community-badge] ![experimental-badge] : Creates the
  base infrastructure for an HTCondor pool (service accounts and Cloud Storage bucket).
* **[htcondor-pool-secrets]** ![community-badge] ![experimental-badge] : Creates
  and manages access to the secrets necessary for secure operation of an
  HTCondor pool.
* **[htcondor-access-point]** ![community-badge] ![experimental-badge] : Creates
  a regional instance group managing a highly available HTCondor access point
  (login node).
* **[pbspro-client]** ![community-badge] ![experimental-badge] : Creates
  a client host for submitting jobs to a PBS Professional cluster.
* **[pbspro-server]** ![community-badge] ![experimental-badge] : Creates
  a server host for operating a PBS Professional cluster.

[batch-job-template]: ../modules/scheduler/batch-job-template/README.md
[batch-login-node]: ../modules/scheduler/batch-login-node/README.md
[gke-cluster]: ../modules/scheduler/gke-cluster/README.md
[pre-existing-gke-cluster]: ../modules/scheduler/pre-existing-gke-cluster/README.md
[htcondor-setup]: ../community/modules/scheduler/htcondor-setup/README.md
[htcondor-pool-secrets]: ../community/modules/scheduler/htcondor-pool-secrets/README.md
[htcondor-access-point]: ../community/modules/scheduler/htcondor-access-point/README.md
[schedmd-slurm-gcp-v6-controller]: ../community/modules/scheduler/schedmd-slurm-gcp-v6-controller/README.md
[schedmd-slurm-gcp-v6-login]: ../community/modules/scheduler/schedmd-slurm-gcp-v6-login/README.md
[slurm-gcp-version-6]: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/6.8.6
[pbspro-client]: ../community/modules/scheduler/pbspro-client/README.md
[pbspro-server]: ../community/modules/scheduler/pbspro-server/README.md

### Scripts

* **[startup-script]** ![core-badge] : Creates a customizable startup script
  that can be fed into compute VMs.
* **[windows-startup-script]** ![community-badge] ![experimental-badge]: Creates
  Windows PowerShell (PS1) scripts that can be used to customize Windows VMs
  and VM images.
* **[htcondor-install]** ![community-badge] ![experimental-badge] : Creates
  a startup script to install HTCondor and exports a list of required APIs
* **[omnia-install]** ![community-badge] ![experimental-badge] ![deprecated-badge] :
  Installs Slurm via [Dell Omnia](https://github.com/dellhpc/omnia) onto a
  cluster of VM instances. _This module has been deprecated and will be removed
  on August 1, 2024_.
* **[pbspro-preinstall]** ![community-badge] ![experimental-badge] : Creates a
  Cloud Storage bucket with PBS Pro RPM packages for use by PBS clusters.
* **[pbspro-install]** ![community-badge] ![experimental-badge] : Creates a
  Toolkit runner to install [PBS Professional][pbspro] from RPM packages.
* **[pbspro-qmgr]** ![community-badge] ![experimental-badge] : Creates a Toolkit
  runner to run common `qmgr` commands when configuring a PBS Pro cluster.
* **[ramble-execute]** ![community-badge] ![experimental-badge] : Creates a
  startup script to execute
  [Ramble](https://github.com/GoogleCloudPlatform/ramble) commands on a target
  VM
* **[ramble-setup]** ![community-badge] ![experimental-badge] : Creates a
  startup script to install
  [Ramble](https://github.com/GoogleCloudPlatform/ramble) on an instance or a
  slurm login or controller.
* **[spack-setup]** ![community-badge] ![experimental-badge] : Creates a startup
  script to install [Spack](https://github.com/spack/spack) on an instance or a
  slurm login or controller.
* **[spack-execute]** ![community-badge] ![experimental-badge] : Defines a
  software build using [Spack](https://github.com/spack/spack).
* **[wait-for-startup]** ![community-badge] ![experimental-badge] : Waits for
  successful completion of a startup script on a compute VM.

[startup-script]: scripts/startup-script/README.md
[windows-startup-script]: ../community/modules/scripts/windows-startup-script/README.md
[htcondor-install]: ../community/modules/scripts/htcondor-install/README.md
[kubernetes-operations]: ../community/modules/scripts/kubernetes-operations/README.md
[omnia-install]: ../community/modules/scripts/omnia-install/README.md
[pbspro-install]: ../community/modules/scripts/pbspro-install/README.md
[pbspro-preinstall]: ../community/modules/scripts/pbspro-preinstall/README.md
[pbspro-qmgr]: ../community/modules/scripts/pbspro-qmgr/README.md
[pbspro]: https://www.altair.com/pbs-professional
[ramble-execute]: ../community/modules/scripts/ramble-execute/README.md
[ramble-setup]: ../community/modules/scripts/ramble-setup/README.md
[spack-setup]: ../community/modules/scripts/spack-setup/README.md
[spack-execute]: ../community/modules/scripts/spack-execute/README.md
[wait-for-startup]: ../community/modules/scripts/wait-for-startup/README.md

> **_NOTE:_** Slurm-GCP V4 is deprecated. In case, you want to use V4 modules, please use
[ghpc-v1.27.0](https://github.com/GoogleCloudPlatform/hpc-toolkit/releases/tag/v1.27.0)
source code and build ghpc binary from this. This source code also contains
deprecated examples using V4 modules for your reference.
> **_NOTE:_** Slurm-GCP V5 is deprecated. In case, you want to use V5 modules, please use
[ghpc-v1.44.1](https://github.com/GoogleCloudPlatform/hpc-toolkit/releases/tag/v1.44.1)
source code and build ghpc binary from this. This source code also contains
deprecated examples using V5 modules for your reference.

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

The source is a path or URL that points to the source files for Packer or
Terraform modules. A source can either be a filesystem path or a URL to a git
repository:

* Filesystem paths
  * modules embedded in the `gcluster` executable
  * modules in the local filesystem
* Remote modules using [Terraform URL syntax](https://developer.hashicorp.com/terraform/language/modules/sources)
  * Hosted on [GitHub](https://developer.hashicorp.com/terraform/language/modules/sources#github)
  * Google Cloud Storage [Buckets](https://developer.hashicorp.com/terraform/language/modules/sources#gcs-bucket)
  * Generic [git repositories](https://developer.hashicorp.com/terraform/language/modules/sources#generic-git-repository)

  when modules are in a subdirectory of the git repository, a special
  double-slash `//` notation can be required as described below

An important distinction is that those URLs are natively supported by Terraform so
they are not copied to your deployment directory. Packer does not have native
support for git-hosted modules so the Toolkit will copy these modules into the
deployment folder on your behalf.

#### Embedded Modules

Embedded modules are added to the gcluster binary during compilation and cannot
be edited. To refer to embedded modules, set the source path to
`modules/<<MODULE_PATH>>` or `community/modules/<<MODULE_PATH>>`.

The paths match the modules in the repository structure for [core modules](./)
and [community modules](../community/modules/). Because the modules are embedded
during compilation, your local copies may differ unless you recompile gcluster.

For example, this example snippet uses the embedded pre-existing-vpc module:

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
    source: modules/network/pre-existing-vpc
```

> **_NOTE:_** Relative paths (beginning with `.` or `..` must be relative to the
> working directory from which `gcluster` is executed. This example would have to be
> run from a local copy of the Cluster Toolkit repository. An alternative is to use
> absolute paths to modules.

#### GitHub-hosted Modules and Packages

To use a Terraform module available on GitHub, set the source to a path starting
with `github.com` (HTTPS) or `git@github.com` (SSH). For instance, the following
module definition sources the Toolkit vpc module:

```yaml
  - id: network1
    source: github.com/GoogleCloudPlatform/hpc-toolkit//modules/network/vpc
```

This example uses the [double-slash notation][tfsubdir] (`//`) to indicate that
the Toolkit is a "package" of multiple modules whose root directory is the root
of the git repository. The remainder of the path indicates the sub-directory of
the vpc module.

The example above uses the default `main` branch of the Toolkit. Specific
[revisions][tfrev] can be selected with any valid [git reference][gitref].
(git branch, commit hash or tag). If the git reference is a tag or branch, we
recommend setting `&depth=1` to reduce the data transferred over the network.
This option cannot be set when the reference is a commit hash. The following
examples select the vpc module on the active `develop` branch and also an older
release of the filestore module:

```yaml
  - id: network1
    source: github.com/GoogleCloudPlatform/hpc-toolkit//modules/network/vpc?ref=develop
  ...
  - id: homefs
    source: github.com/GoogleCloudPlatform/hpc-toolkit//modules/file-system/filestore?ref=v1.22.1&depth=1
```

Because Terraform modules natively support this syntax, gcluster will not copy
GitHub-hosted modules into your deployment folder. Terraform will download them
into a hidden folder when you run `terraform init`.

[tfrev]: https://www.terraform.io/language/modules/sources#selecting-a-revision
[gitref]: https://git-scm.com/book/en/v2/Git-Tools-Revision-Selection#_single_revisions
[tfsubdir]: https://www.terraform.io/language/modules/sources#modules-in-package-sub-directories

##### GitHub-hosted Packer modules

Packer does not natively support GitHub-hosted modules so `gcluster create` will
copy modules into your deployment folder.

If the module uses `//` package notation, `gcluster create` will copy the entire
repository to the module path: `deployment_name/group_name/module_id`. However,
when `gcluster deploy` is invoked, it will run Packer from the subdirectory
`deployment_name/group_name/module_id/subdirectory/after/double_slash`.

If the module does not use `//` package notation, `gcluster create` will copy
only the final directory in the path to `deployment_name/group_name/module_id`.

In all cases, `gcluster create` will remove the `.git` directory from the packer
module to ensure that you can manage the entire deployment directory with its
own git versioning.

##### GitHub over SSH

Get module from GitHub over SSH:

```yaml
  - id: network1
    source: git@github.com:GoogleCloudPlatform/hpc-toolkit.git//modules/network/vpc
```

Specific versions can be selected as for HTTPS:

```yaml
  - id: network1
    source: git@github.com:GoogleCloudPlatform/hpc-toolkit.git//modules/network/vpc?ref=v1.22.1&depth=1
```

##### Generic Git Modules

To use a Terraform module available in a non-GitHub git repository such as
gitlab, set the source to a path starting `git::`. Two Standard git protocols
are supported, `git::https://` for HTTPS or `git::git@github.com` for SSH.

Additional formatting and features after `git::` are identical to that of the
[GitHub Modules](#github-modules) described above.

#### Google Cloud Storage Modules

To use a Terraform module available in a Google Cloud Storage bucket, set the source
to a URL with the special `gcs::` prefix, followed by a [GCS bucket object URL](https://cloud.google.com/storage/docs/request-endpoints#typical).

For example:  `gcs::https://www.googleapis.com/storage/v1/BUCKET_NAME/PATH_TO_MODULE`

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

The order of precedence that `gcluster` uses in determining when to infer a setting
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
printing instructions on how to use a module, as we have in the
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
in the project used by the AI/ML and HPC environment. For example, the [creation of
VMs](compute/vm-instance/) requires the Compute Engine API
(compute.googleapis.com). The [startup-script](scripts/startup-script/) module
requires the Cloud Storage API (storage.googleapis.com) for storage of the
scripts themselves. Each module included in the Toolkit source code describes
its required APIs internally. The Toolkit will merge the requirements from all
modules and [automatically validate](../README.md#blueprint-validation) that all
APIs are enabled in the project specified by `$(vars.project_id)`.

## Common Settings

The following common naming conventions should be used to decrease the verbosity
needed to define a blueprint. This is intentional to allow multiple
modules to share inferred settings from deployment variables or from other
modules listed under the `use` field.

For example, if all modules are to be created in a single region, that region
can be defined as a deployment variable named `region`, which is shared between
all modules without an explicit setting. Similarly, if many modules need to be
connected to the same VPC network, they all can add the vpc module ID to their
`use` list so that `network_self_link` would be inferred from that vpc module rather
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
* **labels**:
  [Labels](https://cloud.google.com/resource-manager/docs/creating-managing-labels)
  added to the module. In order to include any module in advanced
  monitoring, labels must be exposed. We strongly recommend that all modules
  expose this variable.

## Writing Custom Cluster Toolkit Modules

Modules are flexible by design, however we define some [best practices](../docs/module-guidelines.md) when
creating a new module meant to be used with the Cluster Toolkit.
