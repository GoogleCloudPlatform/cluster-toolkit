## Description

This document provides information on how to deploy an instance of [Sycomp Intelligent Data Storage Platform](https://sycomp.com/solution/hpc/storage/) on Google Cloud Platform ([GCP](https://cloud.google.com/)) using the Google Cluster Toolkit.

> **_NOTE:_**
> Sycomp Storage on GCP does not require an HPC Toolkit wrapper.
> Terraform modules are sourced directly from GitLab.

Terraform modules for Sycomp Intelligent Data Storage Platform are downloaded on deployment using the Google Cloud Toolkit.

The Terraform module parameters are documented in the `README.md` files in the respective module directories of the source GitLab repository. The main modules are:

- `sycomp-scale`
- `sycomp-scale-expansion`

## Examples

The community examples folder (community/examples/sycomp/) contains four example blueprints that you can use to deploy or expand a Sycomp Storage cluster.

- [community/examples/sycomp/sycomp-storage.yaml][sycomp-storage-yaml] -
  Blueprint for deploying a Sycomp Storage cluster consisting of 3 storage servers.

- [community/examples/sycomp/sycomp-storage-expansion.yaml][sycomp-storage-expansion-yaml] -
  Blueprint for expanding the above created cluster from 3 to 4 storage servers.

- [community/examples/sycomp/sycomp-storage-ece.yaml][sycomp-storage-ece-yaml] -
  Blueprint for deploying a Sycomp Storage cluster consisting of 7 storage servers with ECE (Erasure Code Edition) software RAID.

- [community/examples/sycomp/sycomp-storage-slurm.yaml][sycomp-storage-slurm-yaml] -
  Blueprint for deploying a Slurm cluster and Sycomp Storage cluster with 3 servers. The Slurm compute nodes are configured as NFS clients and have the ability to use the Sycomp Storage filesystem.

[sycomp-storage-yaml]: ../../../examples/sycomp/sycomp-storage.yaml
[sycomp-storage-expansion-yaml]: ../../../examples/sycomp/sycomp-storage-expansion.yaml
[sycomp-storage-ece-yaml]: ../../../examples/sycomp/sycomp-storage-ece.yaml
[sycomp-storage-slurm-yaml]: ../../../examples/sycomp/sycomp-storage-slurm.yaml
