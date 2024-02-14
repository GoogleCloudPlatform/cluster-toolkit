# HPC Toolkit Intel Select Solution Cluster Deployment

HPC Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

This tutorial will walk you through deploying an HPC cluster that is based on
the [HPC virtual machine (VM) image](https://cloud.google.com/compute/docs/instances/create-hpc-vm)
and comply to the [Intel Select Solution for Simulation and Modeling criteria](https://www.intel.com/content/www/us/en/products/solutions/select-solutions/hpc/simulation-modeling.html).

[Click here for more information](https://cloud.google.com/compute/docs/instances/create-intel-select-solution-hpc-clusters).

## Select a Project

Select a project in which to deploy an HPC cluster on Google .

<walkthrough-project-setup billing="true"></walkthrough-project-setup>

Once you have selected a project, click START.

## Enable APIs & Permissions

*Skip this step if you already ran this as part of a previous tutorial.*

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

gcloud iam service-accounts enable --project <walkthrough-project-id/> $PROJECT_NUMBER-compute@developer.gserviceaccount.com

gcloud projects add-iam-policy-binding <walkthrough-project-id/> --member=serviceAccount:$PROJECT_NUMBER-compute@developer.gserviceaccount.com --role=roles/editor
```

## Build the Toolkit Binary

*Skip this step if you already ran this as part of a previous tutorial.*

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

## Generate a Deployment

This tutorial will use the blueprint docs/tutorials/intel-select/hpc-cluster-intel-select.yaml, which should be open in the Cloud Shell Editor (on the left).

This file describes the cluster you will deploy. It contains:

* a new network
* a filestore instance
* a custom startup script for the slurm controller
* a custom startup script for the slurm login and compute nodes
* a Slurm cluster with Intel software components pre-installed throughout
  * a Slurm login node
  * a Slurm controller
  * several auto-scaling Slurm partitions

Do you notice the difference between this blueprint and the hpc-slurm example?

After you have inspected the file, use the ghpc binary to create a deployment folder by running:

```bash
./ghpc create --vars project_id=<walkthrough-project-id/> docs/tutorials/intel-select/hpc-cluster-intel-select.yaml
```

> **_NOTE:_** The `--vars` argument is used to override `project_id` in the
> deployment variables.

This will create a deployment directory named `hpc-intel-select/`, which
contains the terraform needed to deploy your cluster.

## Deploy the Cluster

Use the following commands to run terraform and deploy your cluster.

```bash
terraform -chdir=hpc-intel-select/primary init
terraform -chdir=hpc-intel-select/primary apply
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

## Waiting for the cluster to be configured

Although the cluster has been successfully deployed, the startup scripts that
install the additional required software take time to complete. Typically, this
can be around 8 minutes on the controller and 2-3 minutes on the login and
compute nodes.

If you see the following message when you SSH into the login node following the
instructions in the next step, you should logout and give more time for the
startup script to complete.

> _`Slurm is currently being configured in the background`_

Running the following command will allow monitoring the startup scripts on the controller:

```bash
gcloud compute instances get-serial-port-output --port 1 --zone us-central1-c --project <walkthrough-project-id/> slurm-hpc-intel-select-controller | grep startup-script
```

And the login node:

```bash
gcloud compute instances get-serial-port-output --port 1 --zone us-central1-c --project <walkthrough-project-id/> slurm-hpc-intel-select-login0 | grep startup-script
```

The following line would indicate that the startup script completed on the controller:
>_`slurm-hpc-intel-select-controller google_metadata_script_runner: startup-script exit status 0`_

## Connecting to the login node

Once the startup script has completed and Slurm reports readiness, connect to the login node.

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

## Run a Job on the Cluster

   **The commands below should be run on the login node.**

1. Create a default ssh key to be able to ssh between nodes:

   ```shell
   ssh-keygen -N '' -f ~/.ssh/id_rsa
   cp ~/.ssh/id_rsa.pub ~/.ssh/authorized_keys
   chmod 0600 ~/.ssh/authorized_keys
   ```

1. Execute the following commands to activate Intel software components and
   allocate machines to run the Intel Cluster Checker:

```shell
export PATH=/apps/intelpython3/bin/:/sbin:/bin:/usr/sbin:/usr/bin:$PATH
source /apps/clck/2019.10/bin/clckvars.sh
source /apps/psxe_runtime/linux/bin/psxevars.sh
salloc -N4 -p compute
```

This may take a minute while Slurm auto-scales to create the nodes. If you are
curious you can refresh the `Compute Engine` > `VM instances` page and see that
additional VMs have been created.

If the allocation fails, try submitting the job to the debug partition,
by removing the `-p compute` parameter to `salloc`. The message `salloc:
PrologSlurmctld failed, job killed` most likely indicates that your project does
not have sufficient quota for C2 instances in your region.

1. Once the allocation is complete, you will be presented with a shell. Run:

```shell
clck -F intel_hpc_platform_compat-hpc-2018.0
```

Notice this job took ~2-3 minutes to start, since all compute nodes have to install the packages at boot time. In a real production system, this would be part of the slurm image (which is also possible with the HPC Toolkit).

Since we used the compute partition, the job ran on [Compute Optimized
instances](https://cloud.google.com/compute/docs/compute-optimized-machines),
using Intel 3.9 GHz Cascade Lake processors and with placement groups enabled.
Nodes will not be re-used across jobs and will be immediately destroyed after
the job is completed.

The outputs of `clck` will be stored in `clck_execution_warnings.log` and `clck_results.log`.

> **_NOTE:_** If the Slurm controller is shut down before the auto-scale nodes
> are destroyed then they will be left running.

## Destroy the Cluster

To avoid incurring ongoing charges we will want to destroy our cluster. Run the
following command in the cloud shell terminal (not in the pop-up):

```bash
terraform -chdir=hpc-intel-select/primary destroy -auto-approve
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
