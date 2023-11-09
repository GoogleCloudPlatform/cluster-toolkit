# Apptainer Solutions for the HPC Toolkit

This example illustrates the use of the [Apptainer](https://apptainer.org/) container system with the HPC Toolkit.

## Builders

While you can easily use existing [Docker](https://hub.docker.com/) with Apptainer it is more efficient to package your code in the [Singularity Image Format](https://apptainer.org/docs/user/latest/definition_files.html) (SIF) using the `apptainer build` command. We demonstrate using the HPC Toolkit to create a _build instance_ that you can use to create SIF images. We also provide a _custom build step_ that will allow you to use [Google Cloud Build](https://cloud.google.com/build?hl=en) to create SIF images on our serverless CI/CD platform.

## Clusters

The most effective way to incorporate Apptainer into you cloud-based HPC systems is to use the HPC Toolkit to create [custom VM images](https://cloud.google.com/compute/docs/images/create-custom) with Apptainer installed that are then used as part of your HPC system. We provide two blueprints that illustrate this process.

## Examples

### Dev

You can use Apptainer to package your development environment to streamline your workflow in a cluster deployed via the HPC Toolkit. We provide examples of 
- packaging a [miniconda](https://docs.conda.io/projects/miniconda/en/latest/) environment using Apptainer and then deploying and using it in a Slurm allocation
- packaging the [VS Code](https://code.visualstudio.com/) IDE, deploying it in a Slurm allocation and connecting to if from your local VS Code IDE

### GPU

Many modern HPC codes take advantage of the massively parallel execution capability GPUs afford. Apptainer provides seamless integration with NVIDIA devices on Google Cloud. We provide an example of packaging and running a simple GPU [hello world](https://developer.nvidia.com/blog/n-ways-to-saxpy-demonstrating-the-breadth-of-gpu-programming-options/) (SAXPY) application.

### MPI

[MPI](https://en.wikipedia.org/wiki/Message_Passing_Interface) is an important component of many HPC applications. Apptainer enables you to package MPI codes along with your chosen MPI runtime, e.g. [Open MPI](https://www.open-mpi.org/), [MPICH](https://www.mpich.org/), etc., and then take advantage Slurm's support for the [Process Management Interface](https://link.springer.com/chapter/10.1007/978-3-642-15646-5_4) (PMI) to execute them independent of the MPI runtime(s) available on the cluster. We demonstrate this capability for both MPICH and Open MPI.

## Demostrations

We provide a set of demonstations of Apptainer's use in larger _real world_ applications
- [OpenRadioss](https://www.openradioss.org/)
- [Multi-GPU Differentiable Modeling](https://github.com/PTsolvers/gpu-workshop-JuliaCon23/tree/main)