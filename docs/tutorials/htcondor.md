# HPC Toolkit HTCondor Tutorial

HPC Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

This tutorial will walk you through deploying a simple HTCondor pool on Google
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

<walkthrough-enable-apis apis="storage.googleapis.com,compute.googleapis.com,secretmanager.googleapis.com"></walkthrough-enable-apis>

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

This tutorial will use community/examples/htcondor-pool.yaml, which provisions
a basic auto-scaling HTCondor pool.

* a new VPC network secured from the public internet
* an HTCondor Access Point for users to submit jobs
* an HTCondor Central Manager that will operate the pool
* a Managed Instance Group to scale a pool of HTCondor Execute Points to serve
  new jobs as they are submitted

The blueprint community/examples/htcondor-pool.yaml should be open in the Cloud
Shell Editor (on the left).

This file describes the cluster you will deploy. After you have inspected the
file, use the ghpc binary to create a deployment directory by running:

```bash
./ghpc create community/examples/htcondor-pool.yaml --vars "project_id=<walkthrough-project-id/>"
```

> **_NOTE:_** The `--vars` argument is used to override `project_id` in the
> blueprint variables. The `--vars` argument supports comma-separated list of
> name=value variables to override blueprint variables. This feature only
> supports variables of string type.

This will create a deployment directory named `htcondor-001/`, which
contains the terraform needed to deploy your cluster.

## Deploy the Cluster

Use the following commands to run terraform and deploy your cluster.

```bash
terraform -chdir=htcondor-001/htcondor init
terraform -chdir=htcondor-001/htcondor validate
terraform -chdir=htcondor-001/htcondor apply -auto-approve
```

If you receive any errors during `apply`, you may re-run it to resolve them.
The deployment will take about 3 minutes. There should be regular status updates
in the terminal. If the `apply` is successful, a message similar to the
following will be displayed:

<!-- Note: Bash blocks give "copy to cloud shell" option.  -->
<!-- "shell" or "text" is used in places where command should not be run in cloud shell. -->

```shell
Apply complete! Resources: xx added, 0 changed, 0 destroyed.
```

## Wait for the pool to be ready

Once terraform has finished, you may SSH to the HTCondor Access Point:

```bash
gcloud compute ssh access-point-0 --tunnel-through-iap --project <walkthrough-project-id/> --zone us-central1-c
```

Alternatively, you may browse to the `access-point-0` VM and click on "SSH" in
the Cloud Console at this address:

```text
https://console.cloud.google.com/compute?project=<walkthrough-project-id/>
```

Once you have command line access to the machine, you must wait for HTCondor and
Docker to complete installation. This will take approximately 5 minutes from
when `terraform apply` finished. When it is complete, a message will be printed
to the screen:

```text
******* HTCondor system configuration complete ********
```

You should also verify that the pool is operational by executing

```bash
condor_status -schedd -autoformat Name
```

and observing output similar to

```text
access-point-0.us-central1-c.c.<walkthrough-project-id/>.internal
```

## Submit an example job

The following commands will copy this job into your home directory and submit it
to the HTCondor pool.

```text
universe       = docker
docker_image   = hello-world
output         = out.$(Cluster)-$(Process)
error          = err.$(Cluster)-$(Process)
log            = log.$(Cluster)-$(Process)
request_cpus   = 1
request_memory = 100MB
queue
```

```bash
cp /var/tmp/helloworld.sub .
condor_submit helloworld.sub
```

The output should resemble

```text
Submitting job(s).
1 job(s) submitted to cluster 1.
```

Run `condor_watch_q` to watch your jobs as they transition from `IDLE` to `RUN`
to `DONE`. This will take several minutes are the pool autoscales to serve your
job.

```bash
condor_watch_q
```

The output should resemble

```text
BATCH   IDLE  RUN  DONE  TOTAL  JOB_IDS
ID: 1     1    -     -      1   1.0
```

When complete, observe the output of your job:

```bash
cat out
```

You should see the output of the Docker [Hello, World][helloworld] image.

[helloworld]: https://hub.docker.com/_/hello-world

## Destroy the Cluster

To avoid incurring ongoing charges we will want to destroy our cluster. Begin by
ensuring that the pool has scaled to 0 execute points:

```bash
condor_status -startd -autoformat Name
```

Once `condor_status` returns empty output (all execute points have scaled down),
you may logout from the access point:

```bash
logout
```

> **_NOTE:_** If terraform destroy is run before HTCondor nodes scale down, you
> may have leave behind some idle VMs. Please wait until `condor_status`
> reports no active nodes in the pool.

You should be returned to the Cloud Shell console. You may then destroy your
HTCondor pool:

```bash
terraform -chdir=htcondor-001/htcondor destroy -auto-approve
```

When complete you should see output similar to:

```shell
Destroy complete! Resources: xx destroyed.
```

## Tutorial Complete

<walkthrough-conclusion-trophy></walkthrough-conclusion-trophy>
