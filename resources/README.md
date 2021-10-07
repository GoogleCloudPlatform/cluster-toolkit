# Resources

This directory contains a set of resources built for the HPC Toolkit. These
resources can be used to define components of an HPC cluster.

## Compute

* **simple-instance**: Creates one or more simple VM instances.

## File System

* **filestore**: Creates a [filestore](https://cloud.google.com/filestore) file
system

* **pre-existing-network-storage**: Used when specifying a pre-existing file
system to be mounted by simple_instances and slurm resources.


## Network

* **vpc**: Creates a [Virtual Private Cloud (VPC)](https://cloud.google.com/vpc)
network with regional subnetworks and firewall rules.

* **pre-existing-vpc**: Connects to a pre-existing VPC network. Useful for
connecting newly built components to an existing network.

## Packer

* **custom-image**: Creates a custom VM Image based on the GCP HPC VM image

## Scripts

* **omnia-install**: Installs SLURM via omnia onto a cluster of compute VMs

* **startup-script**: Creates a customizable startup script that can be fed into
compute VMS

* **wait-for-startup**: Waits for successful completion of a startup script on a
compute VM

## Third Party

### Compute

* **SchedMD-slurm-on-gcp-partition**: Creates a SLURM partition that can be used
by the SchedMD-slurm_on_gcp_controller.

### Schduler

* **SchedMD-slurm-on-gcp-controller**: Creates a SLURM controller node using
[slurm-gcp](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/controller)

* **SchedMD-slurm-on-gcp-login-node**: Creates a SLURM login node using
[slurm-gcp](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/login)
