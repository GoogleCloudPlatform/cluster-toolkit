
# Integration Test Configs
This directory contains a set of test YAML files that can be fed into gHPC
to create a blueprint. These configs are used to run integration tests against
`ghpc`. These configs can also be used independently and locally to verify a
local `ghpc` build.

## Config Descriptions
**hpc-cluster-simple.yaml**: Creates a simple cluster with a single compute VM,
filestore as a /home directory and a network. This has been used as a demo
config when presenting the toolkit.

**hpc-cluster-high-io-remote-state.yaml**: Creates a cluster with high
performance IO system with all Terraform state stored remotely.

**hpc-cluster-2filestore-4s_instance.yaml**: A slightly more complicated HPC
cluster that includes 2 filestore (/home and /shared), two license servers, a
head-node and 2 compute vms

**hpc-cluster-slurm.yaml**: Creates a basic auto-scaling SLURM cluster with 2
SLURM patitions and primarily default settings. The blueprint also creates a new
VPC network, a filestore instance mounted to `/home` and a workstation VM.

**omnia-cluster-simple.yaml**: Creates a SLURM cluster using
[DellHPC Omnia](https://github.com/dellhpc/omnia). The cluster is comprised of
one manager node and eight compute nodes that share a `/home` mounted filestore
instance. The pre-existing default VPC network is used.

**instance_with_startup.yaml**: Creates a simple cluster with one
simple-instance and filestore using the startup-script resource to setup and
mount the filestore instance.

**packer.yaml**: Creates a network for Packer to create a custom VM image.
