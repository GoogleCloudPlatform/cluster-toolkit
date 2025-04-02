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

# Use Cluster Toolkit to create a Slurm cluster

The A3 Ultra and A4 machine profiles have a complex build. The details are provided here:

[Create a Slurm cluster](https://cloud.google.com/ai-hypercomputer/docs/create/create-slurm-cluster)

Follow the instructions described in the document list above above to
create the A3 Ultra or A4 Slurm cluster.

## Run cuQuantum on Google Cloud

Running the cuQuantum platform on Google Cloud using the Cluster Toolkit requires a few steps.

## Connect to Slurm
The remaining steps in this tutorial will all be run on the Slurm cluster login node. SSH is used to connect to the login node, and `gcloud` offers an option for SSH connections.
Cloud Console method:

## Go to the Compute Engine > VM instances page.
1. Go to VM instances
1. Connect to the login VM using SSH-in-browser.
1. From the Connect column of the VM, click SSH. Authorize SSH permissions when prompted.

## Command Line method:
Use the following command to ssh into the controller node from cloud shell:
```
gcloud compute ssh $(gcloud compute instances list --filter "name ~ login" --format "value(name)") --tunnel-through-iap --zone $(gcloud compute instances list --filter "name ~ login" --format "value(zone)")
```
You may be prompted to set up SSH. If so follow the prompts and if asked for a password, just hit [enter] leaving the input blank.
After logging in, you may see the following output on the terminal, followed by a terminal prompt:

**Output (do not copy)** 
```
*** Slurm is currently being configured in the background. ***

** WARNING **: The Cluster Toolkit startup scripts are currently running.
```

If you do not see this text, skip to the next step. 
If you do see this text, wait for the following message, then disconnect and reconnect to the login node (reload the page if using the Cloud Console method).

**Output (do not copy)**
```
* NOTICE **: The Cluster Toolkit startup scripts have finished running successfully.
*** Slurm login setup complete ***                                                                               
/home on the controller was mounted over the existing /home.
Log back in to ensure your home directory is correct.
```
From the command line of the VM, run the sinfo command to view the available partition and node information to run our jobs, and confirm that Slurm is operational.
```
sinfo
```

You should see output similar to the following, which shows the Slurm partitions and partition information:
```
PARTITION AVAIL  TIMELIMIT  NODES  STATE NODELIST
a4high*      up   infinite      2   idle a4h0-a4highnodeset-[0-1]
```

## Download batch job files and Dockerfile from Github

To submit a job on Slurm, a Slurm Batch script must be created.
The Slurm batch jobs files and Docker file to run the cuQuantum simulations can be downloaded using `wget`

```
wget https://raw.githubusercontent.com/jrossthomson/cluster-toolkit/refs/heads/develop/examples/quantum-simulation/build_image.sh
wget https://raw.githubusercontent.com/jrossthomson/cluster-toolkit/refs/heads/develop/examples/quantum-simulation/submit.sh
wget https://raw.githubusercontent.com/jrossthomson/cluster-toolkit/refs/heads/develop/examples/quantum-simulation/Dockerfile
```

## Submit the Slurm job to create the updated cuQuantum `enroot` image
[Enroot](https://github.com/NVIDIA/enroot) is an NVIDIA platform to run traditional containers
in unprivileged sandboxes. Here, a Slurm job runs to create the enroot "sqsh" file.
The command to submit a job with Slurm is [sbatch](https://slurm.schedmd.com/sbatch.html). 

Submit the job.
```
sbatch build_image.sh
```
The command to see the jobs in the Slurm batch queue is [squeue](https://slurm.schedmd.com/squeue.html)
```
squeue
```
The output lists running and pending jobs.
```
             JOBID PARTITION     NAME     USER ST       TIME  NODES NODELIST(REASON)
                 1    a4high    build drj_gcp_  R       0:02      1 a3h0-a4highnodeset
```
This may take several minutes to complete.

## Run the cuQuantum container to simulate the circuit
Once the "build_image.sh" step is completed, you can run the `cuquantum-gcp+24.08.sqsh` image 
to simulate a quantum circuit.
 
 Submit the job.
```
sbatch submit.sh
```
Once again, you can see the running job.
```
squeue
```

# View the output

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

## Discussion

The tutorial demonstrated how to run the NVIDIA cuQuantum container to simulate a quantum circuit.

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


