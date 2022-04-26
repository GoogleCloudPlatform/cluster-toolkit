# Example Configs

This directory contains a set of community example YAML files that can be fed
into gHPC to create a blueprint. For more information on how to read, write and
configure a custom blueprint, see
[the core examples folder](../../examples/README.md).

## Config Descriptions

### spack-gromacs.yaml

Spack is a HPC software package manager. This example creates a small slurm
cluster with software installed with
[Spack](../resources/scripts/spack-install/README.md) The controller will
install and configure spack, and install [gromacs](https://www.gromacs.org/)
using spack. Spack is installed in a shared location (/apps) via filestore. This
build leverages the startup-script resource and can be applied in any cluster by
using the output of spack-install or startup-script resources.

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

### omnia-cluster.yaml

Creates a simple omnia cluster, with an
omnia-manager node and 2 omnia-compute nodes, on the pre-existing default
network. Omnia will be automatically installed after the nodes are provisioned.
All nodes mount a filestore instance on `/home`.
