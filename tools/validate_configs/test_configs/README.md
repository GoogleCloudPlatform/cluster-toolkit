
# Integration Test Blueprints

This directory contains a set of test blueprint files that can be fed into gHPC
to create a deployment. These blueprints are used to run integration tests
against `gcluster`. These blueprints can also be used independently and locally to
verify a local `gcluster` build.

## Blueprint Descriptions

**2-nfs-servers.yaml**: Creates 2 NFS servers with different local mount points,
but otherwise the same variables. This test exists to ensure there will be no
naming collisions when more than one NFS server is created in a projects with
the same deployment name.

**gpu.yaml**: Deploys a set of VM instances (`vm-instance`) with different GPU
configurations attached. Both automatic (via `gpu_definition.yaml`) and manually
supplied guest accelerators are adding to the VM instances.

**hpc-cluster-simple.yaml**: Creates a simple cluster with a single compute VM,
filestore as a /home directory and a network. This has been used as a demo
blueprint when presenting the toolkit.

**hpc-cluster-2filestore-4s_instance.yaml**: A slightly more complicated HPC
cluster that includes 2 filestore (/home and /shared), two license servers, a
head-node and 2 compute vms

**hpc-cluster-slurm.yaml**: Creates a basic auto-scaling SLURM cluster with 2
SLURM partitions and primarily default settings. The blueprint also creates a new
VPC network, a filestore instance mounted to `/home` and a workstation VM.

**service-account-profiles.yaml**: Exercises community/modules/iam/service-account-profiles
once per profile (slurm-controller with every capability flag, slurm-login,
slurm-compute, image-builder, and custom), including every resource-scoped
binding type, actAs grants from the controller to the compute accounts, a
custom profile for the Open Front End web identity, a custom profile binding
an existing custom role alongside a generated one, a custom role at a
non-default `custom_role_stage`, and capability flags on the custom profile, with each profile
block connected to its account by `use:`.

**instance_with_startup.yaml**: Creates a simple cluster with one
vm-instance and filestore using the startup-script module to setup and
mount the filestore instance.

**packer-v5-legacy.yaml**: Creates a network for Packer to create a custom VM image.
