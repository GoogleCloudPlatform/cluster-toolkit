
# Integration Test Blueprints

This directory contains a set of test blueprint files that can be fed into gHPC
to create a deployment. These blueprints are used to run integration tests
against `ghpc`. These blueprints can also be used independently and locally to
verify a local `ghpc` build.

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

**omnia-cluster-simple.yaml**: Creates a SLURM cluster using
[DellHPC Omnia](https://github.com/dellhpc/omnia). The cluster is comprised of
one manager node and eight compute nodes that share a `/home` mounted filestore
instance. The pre-existing default VPC network is used.

**instance_with_startup.yaml**: Creates a simple cluster with one
vm-instance and filestore using the startup-script module to setup and
mount the filestore instance.

**packer.yaml**: Creates a network for Packer to create a custom VM image.
