# HPC Toolkit Basic Cluster Deployment

HPC Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

This tutorial will walk you through deploying a simple HPC cluster on Google
Cloud using the HPC Toolkit.

## Select a Project

Select a project in which to deploy an HPC cluster on Google .

<walkthrough-project-setup billing="true"></walkthrough-project-setup>

Once you have selected a project, click START.

## Add Credits to the Project

Talk with your tutorial leader to see if Google Cloud credits are available.

## Enable APIs

In a new Google Cloud project there are several apis that must be enabled to
deploy your HPC cluster. These will be caught when you perform `terraform apply`
but you can save time by enabling them now by running:

```bash
gcloud services enable --project {{project-id}} file.googleapis.com compute.googleapis.com 
```

<!-- Tried the native way to do this and it timed out. Leaving comment here for future reference. -->
<!-- <walkthrough-enable-apis apis="file.googleapis.com,compute.googleapis.com"></walkthrough-enable-apis> -->

## Build the Toolkit Binary

To build HPC Toolkit binary from source run:

```bash
make
```

You should now have a binary named ghpc in the current directory. To verify the
build run:

```bash
./ghpc --version
```

This should show you the version of the HPC Toolkit you are using.

## Generate a Blueprint

To create a blueprint, an input YAML file needs to be written or adapted from
one of the examples found in the `examples/` directory.

This tutorial will use examples/hpc-cluster-small.yaml, which is a good starting
point and creates a blueprint containing:

* a new network
* a filestore instance
* a Slurm login node
* a Slurm controller
* several auto-scaling Slurm partitions

The blueprint examples/hpc-cluster-small.yaml should be open in the Cloud Shell
Editor (on the left).

This file describes the cluster you will deploy. After you have inspected the
file, use the ghpc binary to create a blueprint by running:

```bash
./ghpc create examples/hpc-cluster-small.yaml --vars "project_id={{project-id}}"
```

> **_NOTE:_** The `--vars` argument is used to override `project_id` in the YAML
> configuration variables.

This will create a blueprint directory named `hpc-cluster-small/`, which
contains the terraform needed to deploy your cluster.

## Deploy the Cluster

Use the following commands to run terraform and deploy your cluster.

```bash
terraform -chdir=hpc-cluster-small/primary init
terraform -chdir=hpc-cluster-small/primary apply
```

The `terraform apply` command will generate a _plan_ that describes the Google
Cloud resources that will be deployed.

You can review the plan and then start the deployment by typing
**`yes [enter]`**.

The deployment will take about 5 minutes. There should be regular status updates
in the terminal.

If the `apply` is successful, a message similar to the following will be
displayed:

<!-- Note: Bash blocks give "copy to cloud shell" option.  -->
<!-- "shell" or "text" is used in places where command should not be run in cloud shell. -->

```shell
Apply complete! Resources: xx added, 0 changed, 0 destroyed.
```

## Run a Job on the Cluster

Once the blueprint has successfully been deployed, take the following steps to
run a job:

1. Open the following URL in a new tab. This will take you to `Compute Engine` >
   `VM instances` in the Google Cloud Console:

<!-- Note: Cannot embed links in Google Cloud tutorial. Tried markdown and html -->

```text
https://console.cloud.google.com/compute
```

<!-- Note: gcloud ssh does not work for cloud shell for google internal projects. -->
<!-- Tutorial opts to use UI instead -->

1. Click on the `SSH` button associated with the `slurm-hpc-small-login0`
   instance.

This will open a separate pop up window with a terminal into our newly created
Slurm login VM.

1. Next you will run the `hostname` command across 3 nodes. Do this by running
   the following command in the shell popup:

```shell
srun -N 3 hostname
```

This may take a minute while Slurm auto-scales to create the nodes. If you are
curious you can refresh the `Compute Engine` > `VM instances` page and see that
additional VMs have been created.

When the job finishes you should see an output similar to:

```shell
$ srun -N 3 hostname
    slurm-hpc-small-compute-0-0
    slurm-hpc-small-compute-0-1
    slurm-hpc-small-compute-0-2
```

Running the same job again will run much faster as Slurm will reuse the nodes.

The auto-scaled nodes will destroy themselves after several minutes.

## Destroy the Cluster

To avoid incurring ongoing charges we will want to destroy our cluster. Run the
following command in the cloud shell terminal (not in the pop-up):

```bash
terraform -chdir=hpc-cluster-small/primary destroy --auto-approve
```

When complete you should see something like:

```shell
Destroy complete! Resources: xx destroyed.
```

> **_NOTE:_** If destroy is run before Slurm shut down the auto-scale nodes then
> they will be left behind and destroy may fail. In this case you can delete the
> VMs manually and rerun the destroy command above.

## Tutorial Complete

<walkthrough-conclusion-trophy></walkthrough-conclusion-trophy>
