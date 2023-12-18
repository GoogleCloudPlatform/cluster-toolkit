# Using Apptainer Containers

These examples cover some common uses of container technology for high performance computing.

### Before you begin
This demonstration assumes you have access to an [Artifact Registry](https://cloud.google.com/artifact-registry) repository and that you have set up the Apptainer custom build step. See [this section](../README.md#before-you-begin) for details.

### Dev

You can use Apptainer to package your development environment to streamline your workflow in a cluster deployed via the HPC Toolkit. We provide examples of 
- packaging a [miniconda](https://docs.conda.io/projects/miniconda/en/latest/) environment using Apptainer and then deploying and using it in a Slurm allocation
- packaging the [VS Code](https://code.visualstudio.com/) IDE, deploying it in a Slurm allocation and connecting to if from your local VS Code IDE

### GPU

Many modern HPC codes take advantage of the massively parallel execution capability GPUs afford. Apptainer provides seamless integration with NVIDIA devices on Google Cloud. We provide an example of packaging and running a simple GPU [hello world](https://developer.nvidia.com/blog/n-ways-to-saxpy-demonstrating-the-breadth-of-gpu-programming-options/) (SAXPY) application.

### MPI

[MPI](https://en.wikipedia.org/wiki/Message_Passing_Interface) is an important component of many HPC applications. Apptainer enables you to package MPI codes along with your chosen MPI runtime, e.g. [Open MPI](https://www.open-mpi.org/), [MPICH](https://www.mpich.org/), etc., and then take advantage Slurm's support for the [Process Management Interface](https://link.springer.com/chapter/10.1007/978-3-642-15646-5_4) (PMI) to execute them independent of the MPI runtime(s) available on the cluster. We demonstrate this capability for both MPICH and Open MPI.