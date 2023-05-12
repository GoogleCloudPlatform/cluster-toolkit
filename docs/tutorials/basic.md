# HPC Toolkit Basic Cluster Deployment

HPC Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

This tutorial will walk you through deploying a simple HPC cluster on Google
Cloud using the HPC Toolkit.

## Select a Project

Select a project in which to deploy an HPC cluster on Google.

<walkthrough-project-setup billing="true"></walkthrough-project-setup>

Once you have selected a project, click START.

## Add Credits to the Project

Talk with your tutorial leader to see if Google Cloud credits are available.

## Enable APIs & Permissions

In a new Google Cloud project there are several apis that must be enabled to
deploy your HPC cluster. These will be caught when you perform `terraform apply`
but you can save time by enabling them now by running:

<walkthrough-enable-apis apis="file.googleapis.com,compute.googleapis.com,serviceusage.googleapis.com"></walkthrough-enable-apis>

We also need to grant the default compute service account project edit access so
the slurm controller can perform actions such as auto-scaling.

<!-- Tried getting PROJECT_NUMBER using <walkthrough-project-number/> but returns empty string. -->

```bash
PROJECT_NUMBER=$(gcloud projects describe <walkthrough-project-id/> --format='value(projectNumber)')

echo "granting roles/editor to $PROJECT_NUMBER-compute@developer.gserviceaccount.com"

gcloud projects add-iam-policy-binding <walkthrough-project-id/> --member=serviceAccount:"$PROJECT_NUMBER"-compute@developer.gserviceaccount.com --role=roles/editor
```

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

(Optional) To install the `ghpc` binary in your home directory under bin,
run the following command:

```bash
make install
exec $SHELL -l
```

## Generate a Deployment

To create a deployment, an input blueprint file needs to be written or adapted
from one of the examples found in the `examples/` or `community/examples`
directories.

This tutorial will use `examples/hpc-slurm.yaml`, which is a good starting
point and creates a deployment containing:

* a new network
* a filestore instance
* a Slurm login node
* a Slurm controller
* several auto-scaling Slurm partitions

The blueprint `examples/hpc-slurm.yaml` should be open in the Cloud Shell
Editor (on the left).

This file describes the cluster you will deploy. After you have inspected the
file, use the ghpc binary to create a deployment directory by running:

```bash
./ghpc create examples/hpc-slurm.yaml --vars "project_id=<walkthrough-project-id/>"
```

> **_NOTE:_** The `--vars` argument is used to override `project_id` in the
> blueprint variables. The `--vars` argument supports comma-separated list of
> name=value variables to override blueprint variables. This feature only
> supports variables of string type.

This will create a deployment directory named `hpc-small/`, which
contains the terraform needed to deploy your cluster.

## Deploy the Cluster

Use the following commands to run terraform and deploy your cluster.

```bash
terraform -chdir=hpc-small/primary init
terraform -chdir=hpc-small/primary apply
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

> **_NOTE:_** This example does not contain any Packer-based modules but for
> completeness, you can use the following command to deploy a Packer-based
> deployment group:
>
> ```shell
> cd <deployment-directory>/<packer-group>/<custom-vm-image>
> packer init .
> packer validate .
> packer build .
> ```

## Run a Job on the Cluster

Once the cluster has successfully been deployed, take the following steps to
run a job:

1. Open the following URL in a new tab. This will take you to `Compute Engine` >
   `VM instances` in the Google Cloud Console:

   <!-- Note: Cannot embed links in Google Cloud tutorial. Tried markdown and html -->

   ```text
   https://console.cloud.google.com/compute?project=<walkthrough-project-id/>
   ```

   <!-- Note: gcloud ssh does not work for cloud shell for google internal projects. -->
   <!-- Tutorial opts to use UI instead -->

1. Click on the `SSH` button associated with the `slurm-hpc-small-login0`
   instance.

   This will open a separate pop up window with a terminal into our newly created
   Slurm login VM.

   > **_NOTE:_** If you see a message saying:
   > _`Slurm is currently being configured in the background`_, then re-launch
   > the pop up after a minute. This gives time for Slurm to become ready.

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

By default, this runs the job on the `debug` partition. See details in
[examples/](examples/README.md#compute-partition) for how to run on the more
performant `compute` partition.

Running the same job again will run much faster as Slurm will reuse the nodes.

The auto-scaled nodes will be automatically destroyed by the Slurm controller if
left idle for several minutes.

> **_NOTE:_** If the Slurm controller is shut down before the auto-scale nodes
> are destroyed then they will be left running.

## Destroy the Cluster

To avoid incurring ongoing charges we will want to destroy our cluster. Run the
following command in the cloud shell terminal (not in the pop-up):

```bash
terraform -chdir=hpc-small/primary destroy -auto-approve
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
