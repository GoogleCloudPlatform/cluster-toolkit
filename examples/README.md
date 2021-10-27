
# Example Configs
This directory contains a set of example YAML files that can be fed into gHPC
to create a blueprint.

## Instructions
Ensure your project_id is set and other deployment variables such as zone and
region are set correctly under `vars` before creating and deploying an example
config.

Please note that global variables defined under `vars` are automatically
passed to resources if the resources have an input that matches the variable name.

## Config Descriptions
**hpc-cluster-small.yaml**: Creates a basic auto-scaling SLURM cluster with a
single SLURM patition and mostly default settings. The blueprint also creates a
new VPC network, and a filestore instance mounted to `/home`.

**hpc-cluster-high-io.yaml**: Creates a slurm cluster with tiered file systems
for higher performance. It connects to the default VPC of the project and
creates two partitions and a login node.

File systems:
* The homefs mounted at `/home` is a default "PREMIUM" tier filestore with 2.5TiB of capacity
* The projectsfs is mounted at `/projects` and is a high scale SSD filestore
instance with 10TiB of capacity.
* The scratchfs is mounted at `/scratch` and is a DDN Exascaler Lustre file
system designed for high IO performance. The capacity is ~10TiB.

### Experimental
**omnia-cluster-simple.yaml**: Creates a simple omnia cluster, with an omnia-manager node and 8 omnia-compute nodes, on the pre-existing default network. Omnia will be automatically installed after the nodes are provisioned. All nodes mount a filestore instance on `/home`.

## Config Schema
A user defined config should follow the following schema:
```
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

# Many resources can be added from local directories.
resource_groups:
- group: groupName
  resources:
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
```
