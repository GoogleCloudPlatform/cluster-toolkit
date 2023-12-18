# OpenFoam Simulation and Visualization

[OpenFOAM](https://www.openfoam.com/) is a very popular tool for running computational fluid dynamics (CFD) simulations. In this 
demostration you will run OpenFOAM across multiple cores and nodes in a Slurm-based HPC system deployed 
via [HPC Toolkit](https://cloud.google.com/hpc-toolkit/docs/overview). Your demo will use one of OpenFOAM's tutorial models, 
the `motorbike` model. To make the demonstration more interesting you will use [Paraview](https://www.paraview.org/) to visualize the
result of the simulation. The visualization will be served from a compute node with an attached GPU
in the HPC system you deploy.

The demonstration will leverage a number of techniques illustrated in various Apptainer [examples](../../examples/).
In particular you will build an [Open MPI](https://www.open-mpi.org/) container configured for PMI2 and use that in combination with the
standard [OpenFOAM container](https://develop.openfoam.com/packaging/containers/-/blob/main/docker/openfoam-run_rocky-template.def?ref_type=heads) 
to create a custom PMI2 enabled OpenFOAM container you will use to run the parallel
simulation portion of the demo. You will also use the `ingestion` technique to pull the standard [kitware/paraview](https://hub.docker.com/r/kitware/paraview)
image from [Docker Hub](https://hub.docker.com/), transform it into a SIF image, and store it in [Artifact Registry](https://cloud.google.com/artifact-registry).

### Before you begin
This demonstration assumes you have access to an [Artifact Registry](https://cloud.google.com/artifact-registry) repository and that you have set up the Apptainer custom build step. See [this section](../../README.md#before-you-begin) for details.

## Containers

You will use [Cloud Build](https://cloud.google.com/build?hl=en) to build four containers as part of this demonstration
- [ompi4-pmi2](./resources/ompi4-pmi2.def) which packages Open MPI built with Slurm PMI support enabled
- [openfoam](./resources/openfoam.yaml) the 2306 development version of OpenFOAM
- [openfoam-pmi2](./resources/openfoam-pmi2.def) OpenFOAM 2306 repackaged to use the Slurm PMI enabled Open MPI runtime
- [paraview](./resources/paraview.yaml) which _ingests_ the OCI ParaView container and stores it in SIF format in Artifact Registry

All of the Apptainer definition files and Cloud Build configurations are in the [resources](./resources/) directory. Change to that directory now
```bash
cd resources
```

If you are in a hurry you can build all the containers at once using the command
```bash
gcloud builds submit --config=containers.yaml .
```

Go back to the parent directory
```bash
cd ..
```

Now you can skip to the [HPC System Deployment](#hpc-system-deployment) section of this demostration.

### OpenFOAM && Open MPI

One of the goals of this demo is to illustrate using Open MPI in a self-contained manner without relying on the HPC system's MPI runtime(s). This approach
simplifies running containerized MPI codes, by eliminating the need to to complex _bind mounts_ from the compute nodes into the container(s), and makes the
solution more portable since it independent of the MPI runtime(s) installed on the HPC system. The [ompi4-pmi2.def](./resources/ompi4-pmi2.def) container
definition adds the `--with-slurm` and `--with -pmi` flags to a standard Open MPI build. The resulting runtime binaries will support the use of [PMI](https://www.mcs.anl.gov/papers/P1760.pdf) to do the necessary _wire-up_ at the beginning of an MPI computation.

All of the Apptainer definition files and Cloud Build configurations are in the [resources](./resources/) directory. Change to that directory now
```bash
cd resources
```

The command
```bash
gcloud builds submit --config=opmi4-mpi2.yaml .
```

builds the PMI-enabled Open MPI runtime container and stores it in Artifact Registry.

Next you build the 2306 development release of OpenFOAM. You will use this container as part of a _multi-stage_ build which substitutes the `ompi4-pmi2` Open MPI 
runtime for the version installed as part of the standard OpenFOAM build. The [openfoam.yaml](./resources/openfoam.yaml) uses `git` to clone the OpenFOAM source
and then builds the runtime in a [Rocky Linux](https://rockylinux.org/) based container.

Build this interim container with the command
```bash
gcloud builds submit --config=openfoam.yaml .
```

Now you are ready to combine the PMI-enabled Open MPI runtime with the OpenFOAM runtime. The [openfoam-pmi2.def](./resources/openfoam-pmi2.def) container
definition uses a _multi-stage_ build to assemble the container from the `ompi4-pmi2` and `openfoam2306` containers. The build first pulls in the PMI-enabled
Open MPI container
```
Bootstrap: oras
From: {{ LOCATION }}/{{ PROJECT_ID }}/{{ REPO }}/ompi4-pmi2:{{ OMPI4_PMI2_VERSION }}
Stage: mpi
```

Next the OpenFOAM container is loaded and the PMI-enabled Open MPI runtime is copied over from the Open MPI container
```
Bootstrap: oras
From: {{ LOCATION }}/{{ PROJECT_ID }}/{{ REPO }}/openfoam2306:{{ OPENFOAM_VERSION }}
Stage: runtime

%files from mpi
    /opt/openmpi /opt/openmpi
```

[Note the use of Apptainer definition file templating to make the container definition configurable]

Finally the `slurm-pmi` package is installed and some environment variable instantiation is configured and the container is ready to go.

You build it using the command
```bash
gcloud builds submit --config=openfoam-pmi2.yaml .
```

### Paraview

You make Paraview available as a SIF image by _ingesting_ the Docker Hub OCI image into Artifact Registry. The [paraview.yaml](./resources/paraview.yaml) config uses
`apptainer` to pull the OCI image from Docker Hub and convert it to SIF and then push the SIF image to Artifact Registry. Hosting containers in Artifact Registry
gives you more control over what users and/or applications can access them and it eliminates the need to convert from OCI to SIF when using them.

Ingest the Paraview OCI container with the command
```bash
gcloud builds submit --config=paraview.yaml .
```

Go back to the parent directory
```bash
cd ..
```

## HPC System Deployment

Use the [OpenFoam blueprint](./hpc/of-demo.yaml) in the `hpc` directory as the basis for your HPC system. It is conifgured to deploy two partitions. 
- A `compute` partition that will dynamically allocate up to 10 [compute-optimized](https://cloud.google.com/compute/docs/compute-optimized-machines#c2_machine_types) C2-Standard-60 instances.
- A `gpu` partition with up to four N1-STANDARD-8 [general-purpose](https://cloud.google.com/compute/docs/general-purpose-machines#n1_machines) instances one of which will be statically allocated when the system is deployed.

Change directories to the `hpc` directory
```bash
cd hpc
```

Use your preferred text editor or the `sed` command below to set the correct PROJECT_ID value in the blueprint.

```bash
sed -i s/_YOUR_GCP_PROJECT_ID_/${PROJECT_ID}/g #PATH TO#/of-demo.yaml
```

Now you can create the deployment artifacts with the command

```bash
./ghpc create #PATH TO#/of-demo.yaml
```

You should see output that looks like

```
To deploy your infrastructure run:

./ghpc deploy ofdemo

Find instructions for cleanly destroying infrastructure and advanced manual
deployment instructions at:

ofdemo/instructions.txt
```

Per the instructions, bring up your HPC system with the command
```bash
./ghpc deploy ofdemo
```

Go back to the parent directory
```bash
cd ..
```

## Simulation

This demonstration uses one of the standard OpenFOAM tutorial examples. Clone the OpenFOAM repo and use the v2306 branch

```bash
git clone -b OpenFOAM-v2306 https://develop.openfoam.com/Development/openfoam.git
```

Switch to the motorbike tutorial directory
```bash
cd openfoam/tutorials/incompressible/simpleFoam/motorBike
```

Create a `decomposeParDict` file that tells OpenFOAM how many MPI ranks to use in its simulation. Here you are specifying 300 subdomains each of which maps to an MPI rank. How many subdomains you specify depends on your GCP C2 CPU quota; if you don't have quota for 300 C2 CPU cores you will need to change the `numberOfSubdomains` value
```bash
cat << EOF > system/decomposeParDict
/*--------------------------------*- C++ -*----------------------------------*\
| =========                 |                                                 |
| \\      /  F ield         | OpenFOAM: The Open Source CFD Toolbox           |
|  \\    /   O peration     | Version:  4.x                                   |
|   \\  /    A nd           | Web:      www.OpenFOAM.org                      |
|    \\/     M anipulation  |                                                 |
\*---------------------------------------------------------------------------*/
FoamFile
{
    version     2.0;
    format      ascii;
    class       dictionary;
    note        "mesh decomposition control dictionary";
    object      decomposeParDict;
}
// * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * //

numberOfSubdomains  300;
method scotch;
EOF
```

Now create a `job.sh` file that specifies the compute environment for the simulation and all the tasks to be carried out as part of the simultion. Note that the product of the values of the `--nodes` and `--ntasks-per-node` must equal the value of the `numberOfSubdomains` setting int the `decomposeParDict` file you created above
```bash
cat << EOF > job.sh
#!/bin/bash
#SBATCH --partition=compute
#SBATCH --nodes=10
#SBATCH --ntasks-per-node=30

mkdir -p constant/triSurface
cp ~/openfoam/tutorials/resources/geometry/motorBike.obj.gz constant/triSurface/
openfoam surfaceFeatureExtract
openfoam blockMesh
openfoam decomposePar

srun --mpi=pmi2 openfoam snappyHexMesh -parallel -overwrite
srun --mpi=pmi2 openfoam topoSet -parallel
ls -d processor* | xargs -I {} rm -rf ./{}/0
ls -d processor* | xargs -I {} cp -r 0.orig ./{}/0

srun --mpi=pmi2 openfoam patchSummary -parallel
srun --mpi=pmi2 openfoam potentialFoam -parallel -writephi
srun --mpi=pmi2 openfoam checkMesh -writeFields '(nonOrthoAngle)' -constant
srun --mpi=pmi2 openfoam simpleFoam -parallel

openfoam reconstructParMesh -constant
openfoam reconstructPar -latestTime
EOF
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

Pull the `openfoam-pmi2` container and put it in `~/bin`
```bash
apptainer pull ${REPOSITORY_URL}/openfoam2306-pmi2:latest
mkdir ~/bin
mv openfoam2306-pmi2_latest.sif ~/bin/openfoam
```

Now you are ready to actually run the simultion with the command
```bash
sbatch job.sh
```

The HPC system will spin up 10 nodes and then begin running the simulation. Even with 300 cores it will take some time to complete the simulation. Once the nodes are up and the job is running you can watch its progress by tailing the slurm-`N`.out file
```bash
tail -f slurm-N.out
```
[where `N` is the Slurm job id of the simulation]

When the job completes, create an empty `motorbike.foam` file for use by the ParaView visualizaton software
```bash
touch motorbike.foam
```

## Visualization

To visualize the results of the simulation you will run the Paraview server on a GPU equipped compute node. On a workstation you will setup an `ssh tunnel`
which the Paraview client will use to connect to the server. The Paraview server is part of the container you ingested above. To get the client go to 
https://www.paraview.org/download/ choose version 5.7 and download the ParaView distribution for your OS (Windows, Linux, or macOS) and install it.

### Server

To start the ParaView server, create an allocation on a compute node in the `gpu` partition with a GPU attached

```bash
salloc --partition=gpu -n4 --gpus-per-node=1
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

Now start the ParaView server with the command
```bash
apptainer exec --nv ${REPOSITORY_URL}/paraview:pv-v5.7.1-egl-py3 /opt/paraview/bin/pvserver
```

You should see output similar to
```
Waiting for client...
Connection URL: cs://ofdemo-gpu-ghpc-0:11111
Accepting connection(s): ofdemo-gpu-ghpc-0:11111
```

### Client

On a workstation, create an `ssh tunnel` using the command
```bash
gcloud compute ssh ofdemo-gpu-ghpc-0 -- -L 11111:localhost:11111
```

Now start the `paraview` client that you downloaded. The first time you start it the client may take a few minutes to come up. Eventually you should
see the ParaView UI:

<img src=./images/pvui.png alt="Paraview UI" width=1024 />

To connect to the ParaView server, choose `Connect` from the `File menu`

<img src=./images/empty-connection-chooser.png alt="Empty Connection Configuration" width=512 />

Now click `Add Server` and configure a server named _ofdemo_ that uses the ssh tunnel you created earlier

<img src=./images/connection-config.png alt="Configure Connection" width=512 />

Click `Configure` and the new server configuration should be available. Choose it and click `Connect`

<img src=./images/connection-chooser.png alt="Chose Connection" width=512 />

The new connection will appear in the `Pipeline Browser`

<img src=./images/ofdemo-connected.png alt="Server Connected" width=256 />

Now you are ready to select the simulation results you want to visualize. Choose `Open` from the `File` menu 

<img src=./images/file-chooser-unselected.png alt="File Chooser" width=512 />

Scroll down and select the empty `motorbike.foam` file you created earlier and click `OK`

<img src=./images/file-chooser.png alt="File Chooser motorbike.foam selected" width=512 />

`motorbike.foam` will appear in the `Pipeline Browser` and the generated regions will appear in the `Mesh Regions` selector

<img src=./images/motorbike-pipeline.png alt="Pipeline Browser" width=256 />

Choose all of the Mesh Regions with a `motorBike_` prefix and click the `Apply` button in the `Properties` tab

<img src=./images/motorbike-mesh-regions.png alt="Motorbike Regions Selected" width=256 />

The ParaView server will render a view of the motorbike that will appear in the UI in a few moments

<img src=./images/motorbike.png alt="Rendered Motorbike" width=1024 />

ParaView is a powerful visualizaton tool but a complete exploration of its capabilities is beyond the scope of this demonstration. We do, however, encourage you to dive into the documentation and tutorial material available online to learn more.

## Teardown

To bring down your HPC system use the command
```bash
./ghpc destroy ofdemo
```