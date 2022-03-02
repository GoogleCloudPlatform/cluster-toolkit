# Example Configs

This directory contains a set of example YAML files that can be fed into gHPC
to create a blueprint.

## Instructions

Ensure your project\_id is set and other deployment variables such as zone and
region are set correctly under `vars` before creating and deploying an example
config.

Please note that global variables defined under `vars` are automatically
passed to resources if the resources have an input that matches the variable name.

## Config Descriptions

### hpc-cluster-small-amd.yaml
This example is identical to [the Intel example below](#hpc-cluster-small-intelyaml)
with the exception that it uses the AMD C2D machine family to provision VMs.

### hpc-cluster-small-intel.yaml

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

### hpc-cluster-high-io-amd.yaml
This example is identical to [the Intel example
below](#hpc-cluster-high-io-intelyaml)
with the exception that it uses the AMD C2D machine family to provision VMs.

### hpc-cluster-high-io-intel.yaml

Creates a slurm cluster with tiered file systems for higher performance. It
connects to the default VPC of the project and creates two partitions and a
login node.

File systems:

* The homefs mounted at `/home` is a default "PREMIUM" tier filestore with
  2.5TiB of capacity
* The projectsfs is mounted at `/projects` and is a high scale SSD filestore
  instance with 10TiB of capacity.
* The scratchfs is mounted at `/scratch` and is a
  [DDN Exascaler Lustre](../resources/third-party/file-system/DDN-EXAScaler/README.md)
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

### Experimental

**omnia-cluster-simple.yaml**: Creates a simple omnia cluster, with an
omnia-manager node and 8 omnia-compute nodes, on the pre-existing default
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
[hpc-cluster-high-io-intel.yaml](./hpc-cluster-high-io-intel.yaml) example config. The
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

## Resources

Resources are the building blocks of an HPC environment. They can be composed to
create complex deployments using the config YAML. Several resources are provided
by default in the [resources](../resources/README.md) folder.

To learn more about how to refer to a resource in a YAML, please consult the
[resources README file.](../resources/README.md)
