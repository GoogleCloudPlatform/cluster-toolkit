# Run Quantum Circuit Simulation on Google Cloud A3

<img src="https://services.google.com/fh/files/misc/hero-heading.jpg" width="400">


This guide provides instructions on how to run quantum circuit simulation on GPUs using the  [Google Cloud Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment), running the [NVIDIA cuQuantum](https://catalog.ngc.nvidia.com/orgs/nvidia/containers/cuquantum-appliance) on [Slurm](https://slurm.schedmd.com/overview.html)

# Getting Started
## Explore costs

In this tutorial, you use several billable components of Google Cloud. 

* Compute Engine
* Filestore
* Cloud Storage

You can evaluate the costs associated to these resources using the [Google Cloud Pricing Calculator](https://cloud.google.com/products/calculator)

## Reserve capacity

To ensure that your workloads have the A4 or A3 Ultra VM resources required for
these instructions, you can create a [future reservation
request](https://cloud.google.com/compute/docs/instances/reservations-overview). With this
request, you can reserve blocks of capacity for a defined duration in the
future. At that date and time in the future, Compute Engine automatically
provisions the blocks of capacity by creating on-demand reservations that you
can immediately consume by provisioning node pools for this cluster.

Do the following steps to request capacity and gather the required information
to create nodes on a specific block within your reservation:

1. [Request capacity](https://cloud.google.com/ai-hypercomputer/docs/request-capacity).

1. To get the name of the blocks that are available for your reservation,
   run the following command:

   ```sh
   gcloud beta compute reservations blocks list <RESERVATION_NAME> \
       --zone=<COMPUTE_ZONE> --format "value(name)"
   ```
   Replace the following:

   * `<RESERVATION_NAME>`: the name of your reservation.
   * `<COMPUTE_ZONE>`: the compute zone of your reservation.

   The output has the following format: <code><var>BLOCK_NAME</var></code>.
   For example the output might be similar to the following: `example-res1-block-0001`.

1. If you want to target specific blocks within a reservation when
   provisioning {{gke_name_short}} node pools, you must specify the full reference
   to your block as follows:

    ```none
   <RESERVATION_NAME>/reservationBlocks/<BLOCK_NAME>
   ```

   For example, using the example output in the preceding step, the full path is as follows: `example-res1/reservationBlocks/example-res1-block-0001`

## Review basic requirements

Some basic items are required to get started.

* A Google Cloud Project with billing enabled.
* Basic familiarity with Linux and command-line tools.

For installed software, you need a few tools.

* [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) installed and configured.
* [Terraform](https://learn.hashicorp.com/tutorials/terraform/install-cli) installed.
* [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git) installed.

> These tools are already installed within the [Google Cloud Shell](https://shell.cloud.google.com/) and Cloud Shell Editor.

## Install the Google Cloud Cluster toolkit

To run the remainder of this tutorial, you must:

* Set up [Cloud Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment#clone-repo). During the setup ensure you enable all the required APIs, and permissions, and grant credentials to Terraform. Also ensure you clone and build the Cloud Cluster Toolkit repository in your local environment.
* Review the [best practices](https://cloud.google.com/cluster-toolkit/docs/tutorials/best-practices).

# Run cuQuantum on Google Cloud

Running the cuQuantum platform on Google Cloud using the Cluster Toolkit requires a few steps.

## Run the Cluster Toolkit blueprint

To build the cuQuantum example Slurm cluster, go to the appropriate directory.
If the Toolkit is installed in the \$HOME directory, the command is:
```
cd ~/cluster-toolkit/examples/quantum-simulation
```
Execute the `gcluster` command. If the Toolkit is installed in the \$HOME directory, the command is:

```
~/cluster-toolkit/gcluster deploy -d a4high-slurm-deployment.yaml \
    ../machine-learning/a4-highgpu-8g/a4high-slurm-blueprint.yaml \
    --skip-validators="test_apis_enabled"  --auto-approve \
    --vars project_id=$(gcloud config get project)
```
## Connect to Slurm
The remaining steps in this tutorial will all be run on the Slurm cluster login node. SSH is used to connect to the login node, and `gcloud` offers an option for SSH connections.
```
gcloud compute ssh --zone "asia-northeast3-a" "namdslurm-slurm-login-001" --project $(gcloud config get project)
```
An alternative to SSH connection to the login node is to connect from the 
[Cloud Console](https://console.cloud.google.com/compute/instances). Click on the `SSH` link.
## Download sample configuration
To run NAMD, configuration files are required. NVIDIA shares information for the APOA1 benchmark. Download the benchmark configuration. 
```
wget -O - https://gitlab.com/NVHPC/ngc-examples/raw/master/namd/3.0/get_apoa1.sh | bash
```
>> For convenience, the deployment has created  download shell script to get this data and the data for STMV. Available on the login node.
```
cp /tmp/get_data.sh .
bash get_data.sh
```

## Convert Docker to Apptainer
[Apptainer](https://apptainer.org/) is recommended for HPC applications. The published 
[NVIDIA Docker Container](https://catalog.ngc.nvidia.com/orgs/hpc/containers/namd)
is easily convereted to Apptainer compatible formats.

`apptainer` has been previously installed on the cluster.

The `apptainer build` command will convert a docker container into apptainer format. The Slurm `sbatch` will
run this step if `namd.sif` is not present, so this step is optional since the `sbatch` file contains 
commands to download and convert the container.
```
export NAMD_TAG=3.0-beta5
apptainer build namd.sif docker://nvcr.io/hpc/namd:$NAMD_TAG 
```
This may take 5 minutes.

## Slurm batch file
To submit a job on Slurm, a Slurm Batch script must be created.

>> For convenience, the deployment created two Slurm batch job files to run these samples.
```
cp /tmp/*.job .
```
## Create the Slurm batch file
Alternatively, you can create the batch file manually.  Use the `heredoc` below. Cut and paste
the follwing into your Slurm login terminal. 

```
tee namd_apoa1.job << JOB
#!/bin/bash
#SBATCH --job-name=namd_ipoa1_benchmark
#SBATCH --partition=a2
#SBATCH --output=%3A/out.txt
#SBATCH --error=%3A/err.txt

# Build SIF, if it doesn't exist
if [[ ! -f namd.sif ]]; then
  export NAMD_TAG=3.0-beta5
  apptainer build namd.sif docker://nvcr.io/hpc/namd:\$NAMD_TAG 
fi
apptainer run --nv namd.sif namd3 +p4 +devices 0,1 +setcpuaffinity apoa1/apoa1_nve_cuda_soa.namd
JOB
```
This creates a Slurm batch file named namd.job

## Submit the job
The command to submit a job with Slurm is [sbatch](https://slurm.schedmd.com/sbatch.html). 

Submit the job.
```
sbatch namd_apoa1.job
```
The command to see the jobs in the Slurm batch queue is [squeue](https://slurm.schedmd.com/squeue.html)
```
squeue
```
The output lists running and pending jobs.
```
             JOBID PARTITION     NAME     USER ST       TIME  NODES NODELIST(REASON)
                 6        a2 namd_ipo drj_gcp_ CF       0:02      1 namdslurm-a2nodeset-0
```
## Review the output
As configured in the `namd_apoa1.job` file, the standard output of the Slurm job is directed to
`###/out.txt`, where `###` is the JOBID. When the job is complete, it will not be visible
in the  `squeue` output and the output files will be present.


You can use `head` to see the start of the output.
```
head 001/out.txt 
```
Shows:
```
==========
== CUDA ==
==========

CUDA Version 12.3.0

Container image Copyright (c) 2016-2023, NVIDIA CORPORATION & AFFILIATES. All rights reserved.

This container image and its contents are governed by the NVIDIA Deep Learning Container License.

```

You can use `tail` to see the end of the output.
```
tail 001/out.txt 
```
Shows:
```
WRITING EXTENDED SYSTEM TO OUTPUT FILE AT STEP 10000
WRITING COORDINATES TO OUTPUT FILE AT STEP 10000
The last position output (seq=-2) takes 0.030 seconds, 0.000 MB of memory in use
WRITING VELOCITIES TO OUTPUT FILE AT STEP 10000
The last velocity output (seq=-2) takes 0.026 seconds, 0.000 MB of memory in use
====================================================

WallClock: 13.387387  CPUTime: 13.058638  Memory: 0.000000 MB
[Partition 0][Node 0] End of program
```
## Discussion

The tutorial demonstrated how to run the NAMD molecular dynamics IPOA1 benchmark 
using NVIDIA GPUs on Google Cloud. The infrastructure was deploye3d by the Cluster Toolkit,
and the NVIDIA container was deployed by Apptainer. 

Slurm was used as a workload manager. Simulation output was viewed in a text file.

# Clean up

To avoid incurring charges to your Google Cloud account for the resources used in this tutorial, either delete the project containing the resources, or keep the project and delete the individual resources.

## Destroy the HPC cluster

To delete the HPC cluster, run the following command:
```
~/cluster-toolkit/gcluster destroy namd-slurm --auto-approve
```
When complete you will see output similar to:

Destroy complete! Resources: xx destroyed.

**CAUTION**: This approach will destroy all content including the fine tuned model.

## Delete the project

The easiest way to eliminate billing is to delete the project you created for the tutorial.

To delete the project:

1. **Caution**: Deleting a project has the following effects:
    * **Everything in the project is deleted.** If you used an existing project for the tasks in this document, when you delete it, you also delete any other work you've done in the project.
    * **Custom project IDs are lost.** When you created this project, you might have created a custom project ID that you want to use in the future. To preserve the URLs that use the project ID, such as an **<code>appspot.com</code></strong> URL, delete selected resources inside the project instead of deleting the whole project.
2. If you plan to explore multiple architectures, tutorials, or quickstarts, reusing projects can help you avoid exceeding project quota limits.In the Google Cloud console, go to the <strong>Manage resources</strong> page. \
[Go to Manage resources](https://console.cloud.google.com/iam-admin/projects)
3. In the project list, select the project that you want to delete, and then click <strong>Delete</strong>.
4. In the dialog, type the project ID, and then click <strong>Shut down</strong> to delete the project.


