# Apptainer Solutions for the HPC Toolkit

This example illustrates the use of the [Apptainer](https://apptainer.org/) container system with the HPC Toolkit.

## Builders

While you can easily use existing [Docker/OCI](https://hub.docker.com/) containers with Apptainer it is more efficient to package your code in the [Singularity Image Format](https://apptainer.org/docs/user/latest/definition_files.html) (SIF) using the `apptainer build` command. We demonstrate using the HPC Toolkit to create a _build instance_ that you can use to create SIF images. We also provide a _custom build step_ that will allow you to use [Google Cloud Build](https://cloud.google.com/build?hl=en) to create SIF images on our serverless CI/CD platform.

### Before you begin
Most of the examples here use Cloud Build to build Apptainer containers, those containers are then stored in an [Artifact Registry](https://cloud.google.com/artifact-registry) repository. Therefore, if you don't already have an Artifact Registry (AR) repository, you should create one as described [here](https://cloud.google.com/artifact-registry/docs/repositories/create-repos#description). The Cloud Build configuration defaults for the examples assume a mulit-region AR repo with the location `us-docker.pkg.dev`. If you create or use a regional AR repo you will need to use the `--substitutions` flag when you submit builds to Cloud Build to change the default location.

In addition to an AR repo to store containers, you will need to create a custom Apptainer `build step` that the example Cloud Build configurations use to build containers. Creation of the custom apptainer build step is described [here](./builders/cloud/README.md#apptainer-build-step).

## Clusters

The most effective way to incorporate Apptainer into you cloud-based HPC systems is to use the HPC Toolkit to create [custom VM images](https://cloud.google.com/compute/docs/images/create-custom) with Apptainer installed that are then used as part of your HPC system. We provide two blueprints that illustrate this process.

## Examples

### Dev

You can use Apptainer to package your development environment to streamline your workflow in a cluster deployed via the HPC Toolkit. We provide examples of 
- packaging a [miniconda](https://docs.conda.io/projects/miniconda/en/latest/) environment using Apptainer and then deploying and using it in a Slurm allocation
- packaging the [VSCode](https://code.visualstudio.com/) IDE, deploying it in a Slurm allocation and connecting to it from your local VSCode IDE

### GPU

Many modern HPC codes take advantage of the massively parallel execution capability GPUs afford. Apptainer provides seamless integration with NVIDIA devices on Google Cloud. We provide an example of packaging and running a simple GPU [hello world](https://developer.nvidia.com/blog/n-ways-to-saxpy-demonstrating-the-breadth-of-gpu-programming-options/) (SAXPY) application.

### MPI

[MPI](https://en.wikipedia.org/wiki/Message_Passing_Interface) is an important component of many HPC applications. Apptainer enables you to package MPI codes along with your chosen MPI runtime, e.g. [Open MPI](https://www.open-mpi.org/), [MPICH](https://www.mpich.org/), etc., and then take advantage Slurm's support for the [Process Management Interface](https://link.springer.com/chapter/10.1007/978-3-642-15646-5_4) (PMI) to execute them independent of the MPI runtime(s) available on the cluster. We demonstrate this capability for both MPICH and Open MPI.

## Demostrations

We provide a demonstation of Apptainer's use in a larger _real world_ application
- [OpenFOAM](./demos/openfoam/)