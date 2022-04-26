# Example Configs

This directory contains a set of example YAML files that can be fed into gHPC
to create a blueprint.

## Instructions

Ensure your project\_id is set and other deployment variables such as zone and
region are set correctly under `vars` before creating and deploying an example
config.

Please note that global variables defined under `vars` are automatically
passed to resources if the resources have an input that matches the variable name.

### (Optional) Setting up a remote terraform state

The following block will configure terraform to point to an existing GCS bucket
to store and manage the terraform state. Add your own bucket name and
(optionally) a service account in the configuration. If not set, the terraform
state will be stored locally within the generated blueprint.

Add this block to the top-level of your input YAML:

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: a_bucket
    impersonate_service_account: a_bucket_reader@project.iam.gserviceaccount.com
```

You can set the configuration at CLI as well like below:

```shell
./ghpc create examples/hpc-cluster-small.yaml --vars "project_id=${GOOGLE_CLOUD_PROJECT}" --backend-config "bucket=${GCS_BUCKET}"
```

> **_NOTE:_** The `--backend-config` argument supports comma-separated list of name=value
> variables to set Terraform Backend configuration in blueprints. This feature only supports
> variables of string type. If you set configuration by both Yaml and CLI, the tool uses values at CLI. "gcs" is set as type by default.

## Config Descriptions

### hpc-cluster-small.yaml

Creates a basic auto-scaling SLURM cluster with mostly default settings. The
blueprint also creates a new VPC network, and a filestore instance mounted to
`/home`.

There are 2 partitions in this example: `debug` and `compute`. The `debug`
partition uses `n2-standard-2` VMs, which should work out of the box without
needing to request additional quota. The purpose of the `debug` partition is to
make sure that first time users are not immediately blocked by quota
limitations.

#### Compute Partition

There is a `compute` partition that achieves higher performance. Any
performance analysis should be done on the `compute` partition. By default it
uses `c2-standard-60` VMs with placement groups enabled. You may need to request
additional quota for `C2 CPUs` in the region you are deploying in. You can
select the compute partition using the `srun -p compute` argument.

Quota required for this example:

* Cloud Filestore API: Basic SSD (Premium) capacity (GB) per region: **2660 GB**
* Compute Engine API: Persistent Disk SSD (GB): **~10 GB**
* Compute Engine API: N2 CPUs: **12**
* Compute Engine API: C2 CPUs: **60/node** up to 1200 - _only needed for
  `compute` partition_
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for `compute` partition_

### hpc-cluster-high-io.yaml

Creates a slurm cluster with tiered file systems for higher performance. It
connects to the default VPC of the project and creates two partitions and a
login node.

File systems:

* The homefs mounted at `/home` is a default "PREMIUM" tier filestore with
  2.5TiB of capacity
* The projectsfs is mounted at `/projects` and is a high scale SSD filestore
  instance with 10TiB of capacity.
* The scratchfs is mounted at `/scratch` and is a
  [DDN Exascaler Lustre](../community/resources/file-system/DDN-EXAScaler/README.md)
  file system designed for high IO performance. The capacity is ~10TiB.

There are two partitions in this example: `low_cost` and `compute`. The
`low_cost` partition uses `n2-standard-4` VMs. This partition can be used for
debugging and workloads that do not require high performance.

Similar to the small example, there is a
[compute partition](#compute-partition) that should be used for any performance
analysis.

Quota required for this example:

* Cloud Filestore API: Basic SSD (Premium) capacity (GB) per region: **2660 GB**
* Cloud Filestore API: High Scale SSD capacity (GB) per region: **10240 GiB** - _min
  quota request is 61440 GiB_
* Compute Engine API: Persistent Disk SSD (GB): **~14000 GB**
* Compute Engine API: N2 CPUs: **158**
* Compute Engine API: C2 CPUs: **60/node** up to 12,000 - _only needed for
  `compute` partition_
* Compute Engine API: Affinity Groups: **one for each job in parallel** - _only
  needed for `compute` partition_
* Compute Engine API: Resource policies: **one for each job in parallel** -
  _only needed for `compute` partition_

### image-builder.yaml

This Blueprint uses the [Packer template resource][pkr] to create custom VM
images by applying software and configurations to existing images. By default,
it uses the [HPC VM Image][hpcimage] as a source image. Using a custom VM image
can be more scalable than installing software using boot-time startup scripts
because

* it avoids reliance on continued availability of package repositories
* VMs will join an HPC cluster and execute workloads more rapidly due to reduced
  boot-time configuration
* machines are guaranteed to boot with a static set of packages available when
  the custom image was created. No potential for some machines to be upgraded
  relative to other based upon their creation time!

[hpcimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm
[pkr]: ../resources/packer/custom-image/README.md

**Note**:  this example relies on the default behavior of the Toolkit to derive
naming convention for networks and other resources from the `deployment_name`.

#### Custom Network (resource group 1)

A tool called [Packer](https://packer.io) builds custom VM images by creating
short-lived VMs, executing scripts on them, and saving the boot disk as an
image that can be used by future VMs. The short-lived VM must operate in a
network that

* has outbound access to the internet for downloading software
* has SSH access from the machine running Packer so that local files/scripts
  can be copied to the VM

This resource group creates such a network, while using [Cloud Nat][cloudnat]
and [Identity-Aware Proxy (IAP)][iap] to allow outbound traffic and inbound SSH
connections without exposing the machine to the internet on a public IP address.

[cloudnat]: https://cloud.google.com/nat/docs/overview
[iap]: https://cloud.google.com/iap/docs/using-tcp-forwarding

#### Toolkit Runners (resource group 1)

The Toolkit [startup-script](../resources/scripts/startup-script/README.md)
module supports boot-time configuration of VMs using "runners." Runners are
configured as a series of scripts uploaded to Cloud Storage. A simple, standard
[VM startup script][cloudstartup] runs at boot-time, downloads the scripts from
Cloud Storage and executes them in sequence.

The standard bash startup script is exported as a string by the startup-script
module.

[vmstartup]: https://cloud.google.com/compute/docs/instances/startup-scripts/linux

#### Packer Template (resource group 2)

The Packer template in this resource group accepts [several methods for
executing custom scripts][pkr]. To pass the exported startup string to it, you
must collect it from the Terraform module and provide it to the Packer template.
After running `terraform -chdir=image-builder/builder-env apply` as instructed
by `ghpc`, execute the following:

```shell
terraform -chdir=image-builder/builder-env \
  output -raw startup_script_install_ansible > \
  image-builder/packer/custom-image/startup_script.sh
cd image-builder/packer/custom-image
packer init .
packer validate -var startup_script_file=startup_script.sh .
packer build -var startup_script_file=startup_script.sh .
```

### (Community) spack-gromacs.yaml

Spack is a HPC software package manager. This example creates a small slurm
cluster with software installed with
[Spack](../community/resources/scripts/spack-install/README.md) The controller
will install and configure spack, and install
[gromacs](https://www.gromacs.org/) using spack. Spack is installed in a shared
location (/apps) via filestore. This build leverages the startup-script resource
and can be applied in any cluster by using the output of spack-install or
startup-script resources.

The installation will occur as part of the slurm startup-script, a warning
message will be displayed upon SSHing to the login node indicating
that configuration is still active. To track the status of the overall
startup script, run the following command on the login node:

```shell
sudo tail -f /var/log/messages
```

Spack specific installation logs will be sent to the spack_log as configured in
your YAML, by default /var/log/spack.log in the login node.

```shell
sudo tail -f /var/log/spack.log
```

Once Slurm and spack installation is complete, spack will available on the login
node. To use spack in the controller or compute nodes, the following command
must be run first:

```shell
source /apps/spack/share/spack/setup-env.sh
```

To load the gromacs module, use spack:

```shell
spack load gromacs
```

 **_NOTE:_** Installing spack compilers and libraries in this example can take 1-2
hours to run on startup. To decrease this time in future deployments, consider
including a spack build cache as described in the comments of the example.

### (Community) omnia-cluster.yaml

Creates a simple omnia cluster, with an
omnia-manager node and 2 omnia-compute nodes, on the pre-existing default
network. Omnia will be automatically installed after the nodes are provisioned.
All nodes mount a filestore instance on `/home`.

## Config Schema

A user defined config should follow the following schema:

```yaml
# Required: Name your blueprint, this will also be the name of the directory
# the blueprint created in.
blueprint_name: MyBlueprintName

# Top-level variables, these will be pulled from if a required variable is not
# provided as part of a resource. Any variables can be set here by the user,
# labels will be treated differently as they will be applied to all created
# GCP resources.
vars:
  project_id: GCP_PROJECT_ID

# https://cloud.google.com/compute/docs/regions-zones
  region: us-central1
  zone: us-central1-a

# https://cloud.google.com/resource-manager/docs/creating-managing-labels
  labels:
    global_label: label_value

# Many resources can be added from local and remote directories.
resource_groups:
- group: groupName
  resources:

  # Local source, prefixed with ./ (/ and ../ also accepted)
  - source: ./resources/role/resource-name # Required: Points to the resource directory.
    kind: < terraform | packer > # Required: Type of resource, currently choose from terraform or packer.
    id: <a unique id> # Required: Name of this resource used to uniquely identify it.
    # Optional: All configured settings for the resource. For terraform, each
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

  # Embedded resource (part of the toolkit), prefixed with resources/
  - source: resources/role/resource-name

  # GitHub resource over SSH, prefixed with git@github.com
  - source: git@github.com:org/repo.git//resources/role/resource-name

  # GitHub resource over HTTPS, prefixed with github.com
  - source: github.com/org/repo//resources/role/resource-name
```

## Writing Config YAML

The input YAML is composed of 3 primary parts, top-level parameters, global variables and resources group. These are described in more detail below.

### Top Level Parameters

* **blueprint_name** (required): Name of this set of blueprints. This also defines the name of the directory the blueprints will be created into.

### Global Variables

```yaml
vars:
  region: "us-west-1"
  labels:
    "user-defined-global-label": "slurm-cluster"
  ...
```

Global variables are set under the vars field at the top level of the YAML.
These variables can be explicitly referenced in resources as
[Config Variables](#config-variables). Any resource setting (inputs) not explicitly provided and
matching exactly a global variable name will automatically be set to these
values.

Global variables should be used with care. Resource default settings with the
same name as a global variable and not explicitly set will be overwritten by the
global variable.

The global “labels” variable is a special case as it will be appended to labels
found in resource settings, whereas normally an explicit resource setting would
be left unchanged. This ensures that global labels can be set alongside resource
specific labels. Precedence is given to the resource specific labels if a
collision occurs. Default resource labels will still be overwritten by global
labels.

The HPC Toolkit uses special reserved labels for monitoring each deployment.
These are set automatically, but can be overridden through global vars or
resource settings. They include:

* ghpc_blueprint: The name of the blueprint the deployment was created from
* ghpc_deployment: The name of the specific deployment of the blueprint
* ghpc_role: The role of a given resource, e.g. compute, network, or
  file-system. By default, it will be taken from the folder immediately
  containing the resource. Example: A resource with the source path of
  `./resources/network/vpc` will have `network` as its `ghpc_role` label by
  default.

### Resource Groups

Resource groups allow distinct sets of resources to be defined and deployed as a
group. A resource group can only contain resources of a single kind, for example
a resource group may not mix packer and terraform resources.

For terraform resources, a top-level main.tf will be created for each resource
group so different groups can be created or destroyed independently.

A resource group is made of 2 fields, group and resources. They are described in
more detail below.

#### Group

Defines the name of the group. Each group must have a unique name. The name will
be used to create the subdirectory in the blueprint directory that the resource
group will be defined in.

#### Resources

Resources are the building blocks of an HPC environment. They can be composed to
create complex deployments using the config YAML. Several resources are provided
by default in the [resources](../resources/README.md) folder.

To learn more about how to refer to a resource in a YAML, please consult the
[resources README file.](../resources/README.md)

## Variables

Variables can be used to refer both to values defined elsewhere in the config
and to the output and structure of other resources.

### Config Variables

Variables in a ghpc config YAML can refer to global variables or the outputs of
other resources. For global and resource variables, the syntax is as follows:

```yaml
vars:
  zone: us-central1-a

resource_groups:
  - group: primary
     resources:
       - source: path/to/resource/1
         id: resource1
         ...
       - source: path/to/resource/2
         ...
         settings:
            key1: $(vars.zone)
            key2: $(resource1.name)
```

The variable is referred to by the source, either vars for global or the
resource ID for resource variables, followed by the name of the value being
referenced. The entire variable is then wrapped in “$()”.

Currently, references to variable attributes and string operations with
variables are not supported.

### Literal Variables

Formally passthrough variables.

Literal variables are not interpreted by `ghpc` directly, but rather for the
underlying resource. Literal variables should only be used by those familiar
with the underlying resource technology (Terraform or Packer); no validation
will be done before deployment to ensure that they are referencing
something that exists.

Literal variables are occasionally needed when referring to the data structure
of the underlying resource. For example, take the
[hpc-cluster-high-io.yaml](./hpc-cluster-high-io.yaml) example config. The
DDN-EXAScaler resource requires a subnetwork self link, which is not currently
an output of either network resource, therefore it is necessary to refer to the
primary network self link through terraform itself:

```yaml
subnetwork_self_link: ((module.network1.primary_subnetwork.self_link))
```

Here the network1 module is referenced, the terraform module name is the same
as the ID in the `ghpc` config. From the module we can refer to it's underlying
variables as deep as we need, in this case the self_link for it's
primary_subnetwork.

The entire text of the variable is wrapped in double parentheses indicating that
everything inside will be provided as is to the resource.

Whenever possible, config variables are preferred over literal variables. `ghpc`
will perform basic validation making sure all config variables are defined
before creating a blueprint making debugging quicker and easier.
