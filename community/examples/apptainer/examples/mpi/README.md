# MPI with Apptainer

[MPI](https://en.wikipedia.org/wiki/Message_Passing_Interface) is an important component of many HPC applications. Apptainer enables you to package MPI codes along with your chosen MPI runtime, e.g. [Open MPI](https://www.open-mpi.org/), [MPICH](https://www.mpich.org/), etc., and then take advantage Slurm's support for the [Process Management Interface](https://link.springer.com/chapter/10.1007/978-3-642-15646-5_4) (PMI) to execute them independent of the MPI runtime(s) available on the cluster.

### Before you begin
This demonstration assumes you have access to an [Artifact Registry](https://cloud.google.com/artifact-registry) repository and that you have set up the Apptainer custom build step. See [this section](../../README.md#before-you-begin) for details.

## Container Definition

This example illustrates the utility of [multi-stage builds](https://apptainer.org/docs/user/latest/definition_files.html#multi-stage-builds). You will create two containers
- [mpich-pmi2](./mpich-pmi2.def) that packages the MPICH runtime into a reusable container
- [mpich-helloworld](./mpich-helloworld.def) which uses `mpich-pmi2` to compile an [MPI code](./mpi_hello_world.c) and provide the MPI runtime, but not the associated development artifacts

The `mpich-pmi2` definition file starts with the latest [Rocky Linux](https://rockylinux.org/) image and then add the necessary development tools and libraries to build and install MPICH

```
%post
    dnf -y group install "Development tools"
    dnf -y install epel-release
    crb enable
    dnf -y install wget
    dnf -y install hwloc hwloc-devel slurm-pmi slurm-pmi-devel
    dnf -y clean all
    wget https://www.mpich.org/static/downloads/4.1.2/mpich-4.1.2.tar.gz
    tar zxvf mpich-4.1.2.tar.gz && rm mpich-4.1.2.tar.gz
    cd mpich-4.1.2
    ./configure --prefix=/opt/mpich/4.1.2 \
      --disable-fortran \
      --with-hwloc   \
      --with-pmi=pmi2 \
      --with-pmilib=slurm \
      --with-slurm-lib=/usr/lib64/slurm \
      --with-slurm-include=/usr/include/slurm
    make -j $(nproc)
    make install
```

Note the use of the `--with-pmi`, and `--with-pmilib` switches which configure the version of PMI to be used as well as the provider, and the `--with-slurm-lib` and `--with-slurm-include` switches which configure the paths to the Slurm libraries and include files.

The `mpich-helloworld` definition file specifies two stages
- `mpi` which copies the [mpi_hello_world.c](./mpi_hello_world.c) code into an interim container image and compiles it
- `runtime` which copies the compiled `mpi_hello_world` binary and the MPICH runtime from the interim image into the final container image and installs the minimum set of packages required.

The `mpich-helloworld` definition file uses the `mpich-pmi2` container as its starting point and it pulls it from ArtifactRegistry.

```
Bootstrap: oras
From: _LOCATION/_PROJECT_ID/_REPOSITORY/mpich-pmi2:_VERSION
```

The `mpi_hello_world.c` code is copied in and compiled in the `mpi` stage

```
%files
    mpi_hello_world.c /usr/local/src/mpich/mpi_hello_world.c

%post
    /opt/mpich/4.1.2/bin/mpicc -o /usr/local/bin/mpi_hello_world /usr/local/src/mpich/mpi_hello_world.c
```

The `runtime` stage starts with latest `Rocky Linux` image then copies the MPICH runtime and compiled `mpi_hello_world` binary (but not the source) into the final container image


```
Bootstrap: docker
From: rockylinux/rockylinux:latest
Stage: runtime

%files from mpi
    /opt/mpich/4.1.2 /opt/mpich/4.1.2
    /usr/local/bin/mpi_hello_world /usr/local/bin/mpi_hello_world
```

Lastly the packages required by Slurm `pmi` are installed on the final image

```
%post
    dnf -y install epel-release
    crb enable
    dnf -y install slurm-pmi librdmacm
    dnf -y clean all
```

Once built the resulting container will be independent of the MPI runtime installed on the HPC system where it is executed and it won't require anyone who uses it to recompile the actual MPI code.

## Container Build

Since the `mpich-helloworld` container depends on the `mpich-pmi2` container `mpich-pmi2` must be built first.

You build the `mpich-pmi2` container and save it to Artifact Registry with the command

```bash
gcloud builds submit --config=mpich-pmi2build.yaml
```

Note that this will used the default values for
- _LOCATION: _*us-docker.pkg.dev*_
- _REPOSITORY: _*sifs*_
- _VERSION: _*latest*_

If you want to change any of these values add the `--substitution` switch to the command above, e.g., to set the version to `1.0`

```bash
gcloud builds submit --config=mpich-pmi2build.yaml --substitutions=_VERSION=1.0
```

Building `mpich-helloworld` is similar

```bash
gcloud builds submit --config=mpich-helloworldbuild.yaml
```

The default values are the same is the use of substitutions.

Note that the `mpich-pmi2` container can be used to build many different application containers. You would only need to rebuild it to use a different release of MPICH.

## Usage

To run the MPI code you packaged in `mpich-helloworld`, deploy a Slurm-based HPC System using the [slurm-apptainer.yaml](../../../cluster/slurm-apptainer.yaml) blueprint following the process described [here](../../../cluster/README.md). Login to the HPC system's login node with the command

```bash
gcloud compute ssh \
  $(gcloud compute instances list \
      --filter="NAME ~ login" \
      --format="value(NAME)") \
  --tunnel-through-iap
```

Set up access to the Artifact Registry repository

```bash
export REPOSITORY_URL=#ARTIFACT REGISTRY REPOSITORY URL# e.g. oras://us-docker.pkg.dev/myproject/sifs
```

```bash
apptainer remote login \
--username=oauth2accesstoken \
--password=$(gcloud auth print-access-token) \ 
${REPOSITORY_URL}
```

Download the `mpich-helloworld` container

```bash
apptainer pull oras://#LOCATION#/#PROJECT_ID#/#REPOSITORY#/mpich-helloworld:latest
INFO:    Downloading oras image
414.0b / 414.0b [=============================================================================================================================] 100 %0s
127.9MiB / 127.9MiB [============================================================================================================] 100 % 199.8 MiB/s 0s
```

It is not strictly necessary to download the container. You could invoke `apptainer run` as part of your Slurm `srun` command but that would cause each of the MPI ranks in your job to pull the container generating unnecessary network traffic and I/O overhead.

Now run the `mpi_hello_world.c` code across multiple nodes and cores with the command

```bash
srun --mpi=pmi2 --ntasks=12 --tasks-per-node=4 --partition=compute ./mpich-helloworld_latest.sif /usr/local/bin/mpi_hello_world
```

The output should be similar to

```
Hello world from processor hpctainer-compute-ghpc-0, rank 1 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-0, rank 0 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-0, rank 3 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-0, rank 2 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-2, rank 8 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-2, rank 10 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-2, rank 9 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-1, rank 5 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-1, rank 4 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-2, rank 11 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-1, rank 6 out of 12 processors
Hello world from processor hpctainer-compute-ghpc-1, rank 7 out of 12 processors
```