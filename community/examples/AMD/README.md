# AMD solutions for the Cluster Toolkit (formerly HPC Toolkit)

> [!NOTE]
> This document uses Slurm-GCP v6. If you want to use Slurm-GCP v5 version you
> scan refer [blueprint](./hpc-amd-slurm-v5-legacy.yaml)

## AMD-Optimized Slurm Cluster

This example provisions a Slurm cluster using the AMD-based Computed Optimized
"c2d" family of VM types. Additionally, it will install
[Spack](https://spack.io) and a script to install the AMD Optimizing C/C++ and
Fortran Compilers ([AOCC]). Installation of AOCC requires acceptance of an
[End User License Agreement][aocceula] as described below.

[AOCC]: https://developer.amd.com/amd-aocc/
[aocceula]: https://developer.amd.com/wordpress/media/files/AOCC_EULA.pdf

### Provisioning the AMD-optimized Slurm cluster

Identify a project to work in and substitute its unique id wherever you see
`<<PROJECT_ID>>` in the instructions below.

### Initial Setup

Before provisioning any infrastructure in this project you should follow the
Toolkit guidance to enable [APIs][apis] and establish minimum resource
[quotas][quotas]. In particular, the following APIs should be enabled

* file.googleapis.com (Cloud Filestore)
* compute.googleapis.com (Google Compute Engine)

You must have available C2D CPU quota (available = maximum - used) in the
region you intend to provision instances:

* `low_cost` partition
  * C2D CPUs: 40
* `compute` partition
  * C2D CPUs: 5600
* `login` and `controller` nodes
  * C2D CPUs: 8
* Total
  * C2D CPUs: 5648

Additionally, the blueprint provisions 2048GB of Filestore instances in the
region.

The quotas are not enforced on a partition until a cluster scales up beyond the
initial CPUs used by the `login` and `controller` nodes. If you do not plan on
using the `compute` partition, you may ignore its quota requirements.

[apis]: https://github.com/GoogleCloudPlatform/hpc-toolkit#enable-gcp-apis
[quotas]: https://github.com/GoogleCloudPlatform/hpc-toolkit#gcp-quotas

### Deploying the Blueprint

Use `gcluster` to provision the blueprint, supplying your project ID:

```shell
gcluster create --vars project_id=<<PROJECT_ID>> hpc-amd-slurm.yaml
```

It will create a directory containing a Terraform module. Follow the printed
instructions to execute Terraform.

### Run an OpenFOAM test suite

Browse to the [Cloud Console][console] and use the SSH feature to access the
Slurm login node. A script has been provisioned which will activate your
OpenFOAM environment and run a test suite of applications. The output of this
test suite will appear in `openfoam_test` under your home directory. To execute
the test suite, run:

```shell
bash /var/tmp/openfoam_test.sh
```

### Complete installation of AOCC

Because AOCC requires acceptance of a license, we advise a manual step to
install AOCC and OpenMPI compiled with AOCC. You can browse to the
[Cloud Console][console] and use the SSH feature to access the login node. To
check if Spack has completed installation, run:

```shell
sudo tail -f /var/log/spack.log
```

You may see a message printed to the screen saying that `/home` has been
remounted and that you should logout and login. Follow its instructions.

Once configuration is complete, install AOCC by running:

```shell
sudo bash /var/tmp/install_aocc.sh
```

Spack will prompt you to accept the AOCC End User License Agreement by opening a
text file containing information about the license. Leave the file unmodified
and write it to disk by typing `:q` as two characters in sequence
([VI help][vihelp]).

Installation of AOCC and OpenMPI will take approximately 15 minutes.

Configure SSH user keys for access between cluster nodes:

```shell
ssh-keygen -N '' -f ~/.ssh/id_rsa
cp ~/.ssh/id_rsa.pub ~/.ssh/authorized_keys
chmod 0600 ~/.ssh/authorized_keys
```

[console]: https://console.cloud.google.com/compute/instances
[vihelp]: https://stackoverflow.com/a/11828573
