# HPC Toolkit - Install and Run Weather Research and Forecasting (WRF) Model on a Slurm Cluster

HPC Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

In this tutorial you will use the HPC Toolkit to:

* Deploy a [Slurm](https://github.com/GoogleCloudPlatform/slurm-gcp#readme) HPC cluster on
  Google Cloud
* Use [Spack](https://spack.io/) to install the Weather Research and Forecasting (WRF) Model application and all of
  its dependencies
* Run a [Weather Research and Forecasting (WRF) Model](https://www.mmm.ucar.edu/weather-research-and-forecasting-model) job on your newly provisioned
  cluster
* Tear down the cluster

Estimated time to complete:
The tutorial takes 2 hr. to complete,
of which 1.5 hr is for installing software
(without cache).

> **_NOTE:_** With a complete Spack cache, the tutorial takes 30 min.

## Select a Project

Select a project in which to deploy an HPC cluster on Google.

<walkthrough-project-setup billing="true"></walkthrough-project-setup>

Once you have selected a project, click START.

## Enable APIs & Permissions

In a new Google Cloud project there are several apis that must be enabled to
deploy your HPC cluster. These will be caught when you perform `terraform apply`
but you can save time by enabling them now by running:

<walkthrough-enable-apis apis="file.googleapis.com,compute.googleapis.com,logging.googleapis.com,serviceusage.googleapis.com"></walkthrough-enable-apis>

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

This tutorial will use the blueprint docs/tutorials/wrfv3/spack-wrfv3.yaml,
which should be open in the Cloud Shell Editor (on the left).

This file describes the cluster you will deploy. It defines:

* a vpc network
* a monitoring dashboard with metrics on your cluster
* a definition of a custom Spack installation
* a startup script that
  * installs ansible
  * installs Spack & Weather Research and Forecasting (WRF) Model using the definition above
  * sets up a Spack environment including downloading an example input deck
  * places a submission script on a shared drive
* a Slurm cluster
  * a Slurm controller
  * An auto-scaling Slurm partition

[This diagram](https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/application_demo/docs/tutorials/application_demo.md#blueprint-diagram)
shows how the different modules relate to each other.

After you have inspected the file, use the ghpc binary to create a deployment
folder by running:

```bash
./ghpc create docs/tutorials/wrfv3/spack-wrfv3.yaml --vars project_id=<walkthrough-project-id/>
```

> **_NOTE:_** The `--vars` argument is used to override `project_id` in the
> deployment variables.

This will create a deployment directory named `spack-wrfv3/`, which
contains the terraform needed to deploy your cluster.

## Deploy the Cluster

Use below command to deploy your cluster.

```bash
./ghpc deploy spack-wrfv3
```

You can also use below command to generate a plan that describes the Google Cloud resources that will be deployed.

```bash
terraform -chdir=spack-wrfv3/primary init
terraform -chdir=spack-wrfv3/primary apply
```

<!-- Note: Bash blocks give "copy to cloud shell" option.  -->
<!-- "shell" or "text" is used in places where command should not be run in cloud shell. -->

```shell
Apply complete! Resources: xx added, 0 changed, 0 destroyed.
```

## Waiting for the cluster to be configured

Although the cluster has been successfully deployed, the startup scripts that
install Spack and Weather Research and Forecasting (WRF) Model take additional
time to complete. When run without a Spack cache, this installation takes about
1.5 hrs (or 6 min with complete cache).

The following command will print logging from the startup script running on the
controller. This command can be used to view progress and check for completion
of the startup script:

```bash
gcloud compute instances get-serial-port-output --port 1 --zone us-central1-c --project <walkthrough-project-id/> spackwrfv3-controller | grep google_metadata_script_runner
```

When the startup script has finished running you will see the following line as
the final output from the above command:
> _`spackwrfv3-controller google_metadata_script_runner: Finished running startup scripts.`_

Optionally while you wait, you can see your deployed VMs on Google Cloud
Console. Open the link below in a new window. Look for
`spackwrfv3-controller`. If you don't
see your VMs make sure you have the correct project selected (top left).

```text
https://console.cloud.google.com/compute?project=<walkthrough-project-id/>
```

## Connecting to the controller node

Once the startup script has completed, connect to the controller node.

Use the following command to ssh into the controller node from cloud shell:

```bash
gcloud compute ssh spackwrfv3-controller --zone us-central1-c --project <walkthrough-project-id/>
```

You may be prompted to set up SSH. If so follow the prompts and if asked for a
password, just hit `[enter]` leaving the input blank.

If the above command succeeded (and you see a Slurm printout in the console)
then **continue to the next page.**

<!-- Note: gcloud ssh does not work for cloud shell for google internal projects. -->

In some organizations you will not be able to SSH from cloud shell. If the above
command fails you can SSH into the VM through the Cloud Console UI using the
following instructions:

1. Open the following URL in a new tab. This will take you to `Compute Engine` >
   `VM instances` in the Google Cloud Console:

   <!-- Note: Cannot embed links in Google Cloud tutorial. Tried markdown and html -->

   ```text
   https://console.cloud.google.com/compute?project=<walkthrough-project-id/>
   ```

1. Click on the `SSH` button associated with the `spackwrfv3-controller`
   instance.

   This will open a separate pop up window with a terminal into our newly
   created Slurm controller VM.

## Run a Job on the Cluster

   **The commands below should be run on the Slurm controller node.**

We will use the submission script (see line 122 of the blueprint) to submit a
Weather Research and Forecasting (WRF) Model job.

1. Make a directory in which we will run the job:

   ```bash
   mkdir test_run && cd test_run
   ```

2. Submit the job to Slurm to be scheduled:

   ```bash
   sbatch /opt/apps/wrfv3/submit_wrfv3.sh
   ```

3. Once submitted, you can watch the job progress by repeatedly calling the
   following command:

   ```bash
   squeue
   ```

The `sbatch` command trigger Slurm to auto-scale up several nodes to run the job.

You can refresh the `Compute Engine` > `VM instances` page and see that
additional VMs are being/have been created. These will be named something like
`spackwrfv3-compute-0`.

When running `squeue`, observe the job status start as `CF` (configuring),
change to `R` (running) once the compute VMs have been created, and finally `CG`
(completing) when job has finished and nodes are spooling down.

When `squeue` no longer shows any jobs the job has finished. The whole job takes
about 5 minutes to run.

> **_NOTE:_** If the allocation fails, the message
> `salloc: PrologSlurmctld failed, job killed` most likely indicates that your
> project does not have sufficient quota for C2 instances in your region. \
> **_NOTE:_** If the Slurm controller is shut down before the auto-scale nodes
> are destroyed then they will be left running.

## Review the output

Several files will have been generated in the `test_run/` folder you created.

The `rsl.out.0000` file has information on the run. You can view this file by
running the following command on the controller node:

```bash
cat rsl.out.0000
```

## View the cluster monitoring dashboard

To view the monitoring dashboard containing metrics on your cluster, open the
following URL in a new tab and click on the dashboard named
`HPC Toolkit Dashboard: spack-wrfv3`.

```text
https://console.cloud.google.com/monitoring/dashboards?project=<walkthrough-project-id/>
```

## Destroy the Cluster

To avoid incurring ongoing charges we will want to destroy our cluster.

For this we need to return to our cloud shell terminal. Run `exit` in the
terminal to close the SSH connection to the controller node:

> **_NOTE:_** If you are accessing the controller node terminal via a separate pop-up
> then make sure to call `exit` in the pop-up window.

```bash
exit
```

Run the following command in the cloud shell terminal to destroy the cluster:

```bash
./ghpc destroy spack-wrfv3
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
