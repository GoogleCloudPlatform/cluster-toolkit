# Tutorials

## Quickstart Tutorial

Find the quickstart tutorial on
[Google Cloud docs](https://cloud.google.com/hpc-toolkit/docs/quickstarts/slurm-cluster).

## Simple Cluster Tutorial

Deploy a simple HPC cluster with the HPC Toolkit in
[cloud shell](https://cloud.google.com/shell) using the
[hpc-cluster-small.yaml](../../examples/hpc-cluster-small.yaml) example.

It is recommended to use the [Quickstart Tutorial](#quickstart-tutorial), which
covers similar material as the Simple Cluster Tutorial and will be replacing
this tutorial in the future.

Click the button below to launch the Simple Cluster Tutorial.

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://shell.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fhpc-toolkit&cloudshell_open_in_editor=examples%2Fhpc-cluster-small.yaml&cloudshell_tutorial=docs%2Ftutorials%2Fbasic.md)

## Intel Select Tutorial

Walks through deploying an HPC cluster that is based on the
[HPC virtual machine (VM) image][hpc-vm-image] and complies to the
[Intel Select Solution for Simulation and Modeling criteria][intel-select].

Click the button below to launch the Intel Select tutorial.

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://shell.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fhpc-toolkit&cloudshell_open_in_editor=docs%2Ftutorials%2Fintel-select%2Fhpc-cluster-intel-select.yaml&cloudshell_tutorial=docs%2Ftutorials%2Fintel-select%2Fintel-select.md)

[hpc-vm-image]: https://cloud.google.com/compute/docs/instances/create-hpc-vm
[intel-select]: https://www.intel.com/content/www/us/en/products/solutions/select-solutions/hpc/simulation-modeling.html

## HTCondor Tutorial

Walk through deploying an HTCondor pool that supports jobs running inside Docker
containers or the base [HPC VM Image][hpc-vm-image].

Click the button below to launch the HTCondor tutorial.

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://shell.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fhpc-toolkit&cloudshell_open_in_editor=community%2Fexamples%2Fhtcondor-pool.yaml&cloudshell_tutorial=docs%2Ftutorials%2Fhtcondor.md)

## Application Specific Tutorials

The following three tutorials deploy a cluster, install an hpc application
(Growmacs, Openfoam, or WRF), and walk through submitting a real workload.

By default these tutorials build the applications from source, which takes
several hours on deployment. If a complete Spack cache is provided using the
`spack_cache_mirror_url` variable, application installation can be reduced to 6
minutes.

### Gromacs

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://shell.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fhpc-toolkit&cloudshell_git_branch=main&cloudshell_open_in_editor=docs%2Ftutorials%2Fgromacs%2Fspack-gromacs.yaml&cloudshell_tutorial=docs%2Ftutorials%2Fgromacs%2Fspack-gromacs.md)

### Openfoam

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://shell.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fhpc-toolkit&cloudshell_git_branch=main&cloudshell_open_in_editor=docs%2Ftutorials%2Fopenfoam%2Fspack-openfoam.yaml&cloudshell_tutorial=docs%2Ftutorials%2Fopenfoam%2Fspack-openfoam.md)

### Weather Research and Forecasting (WRF) Model

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://shell.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2FGoogleCloudPlatform%2Fhpc-toolkit&cloudshell_git_branch=main&cloudshell_open_in_editor=docs%2Ftutorials%2Fwrfv3%2Fspack-wrfv3.yaml&cloudshell_tutorial=docs%2Ftutorials%2Fwrfv3%2Fspack-wrfv3.md)

### Blueprint Diagram for Application Tutorials

All the application specific tutorials above use similar blueprints built of
from a number of terraform modules. The diagram below shows how the various
modules relate to each other.

```mermaid
graph TB
    A(Virtual Private Cloud) 
    C(Spack Install Script)
    D(Startup Scripts)
    E(Compute Partition)
    F(Slurm Controller) 
    G(Slurm Login Node)
    B(Monitoring Dashboard)
    C --> D
    A --> E
    A --> F
    E --> F
    D --> F
    A --> G
    F --> G
```
