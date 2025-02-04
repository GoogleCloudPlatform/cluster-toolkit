# Cluster Toolkit HTCondor Tutorial

Cluster Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

This tutorial will walk you through deploying a simple HTCondor pool on Google
Cloud using the Cluster Toolkit.

## Select a Project

Select a project in which to deploy an HPC cluster on Google.

<walkthrough-project-setup billing="true"></walkthrough-project-setup>

Once you have selected a project, click START.

## Add Credits to the Project

Talk with your tutorial leader to see if Google Cloud credits are available.

## Enable APIs & Permissions

In a new Google Cloud project there are several APIs that must be enabled to
deploy your HPC cluster. These will be caught when you perform `terraform apply`
but you can save time by enabling them now by running:

<walkthrough-enable-apis apis="storage.googleapis.com,compute.googleapis.com,secretmanager.googleapis.com,serviceusage.googleapis.com,cloudresourcemanager.googleapis.com,iam.googleapis.com,logging.googleapis.com"></walkthrough-enable-apis>

## Build the Toolkit Binary

To build Cluster Toolkit binary from source run:

```bash
make
```

You should now have a binary named gcluster in the current directory. To verify the
build run:

```bash
./gcluster --version
```

This should show you the version of the Cluster Toolkit you are using.

(Optional) To install the `gcluster` binary in your home directory under bin,
run the following command:

```bash
make install
exec $SHELL -l
```

## Generate a Deployment

To create a deployment, an input blueprint file needs to be written or adapted
from one of the examples found in the `examples/` or `community/examples`
directories.

This tutorial will use `community/examples/htc-htcondor.yaml`, which provisions
a basic auto-scaling HTCondor pool.

* a new VPC network secured from the public internet
* an HTCondor Access Point for users to submit jobs
* an HTCondor Central Manager that will operate the pool
* 2 Managed Instance Groups for HTCondor Execute Points: 1 is configured with
  Spot pricing and the other with On-Demand pricing

The blueprint `community/examples/htc-htcondor.yaml` should be open in the Cloud
Shell Editor (on the left).

This file describes the cluster you will deploy. After you have inspected the
file, use the gcluster binary to create a deployment directory by running:

```bash
./gcluster create community/examples/htc-htcondor.yaml --vars "project_id=<walkthrough-project-id/>"
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
./gcluster deploy htcondor-pool --auto-approve
```

The Toolkit will automatically approve provisioning a network, building a VM
image with HTCondor and, finally, the HTCondor pool itself. There will be
regular status updates in the terminal. At the conclusion, a message similar to
the following will be displayed:

<!-- Note: Bash blocks give "copy to cloud shell" option.  -->
<!-- "shell" or "text" is used in places where command should not be run in cloud shell. -->

```shell
Apply complete! Resources: xx added, 0 changed, 0 destroyed.
```

## Wait for the pool to be ready

Once terraform has finished, you may SSH to the HTCondor Access Point:

```bash
gcloud compute ssh htcondor-pool-ap-0 --tunnel-through-iap --project <walkthrough-project-id/> --zone us-central1-c
```

Alternatively, you may browse to the `htcondor-pool-ap-0` VM and click on "SSH" in
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

Until HTCondor is installed and configured, you may find that this command is
not yet available ("command not found") or fails to find the pool ("Failed to
connect"). Installation may take 5 minutes or more. When it succeeds, you will
observe output similar to

```text
htcondor-pool-ap-0.us-central1-c.c.<walkthrough-project-id/>.internal
```

## Submit an example job

An example job is automatically copied to your HTCondor access point. The
following commands will copy the example into your home directory and submit it
to the pool.

```bash
cp /var/tmp/helloworld.sub .
condor_submit helloworld.sub
```

The job "submit file" will resemble:

```text
universe       = vanilla
executable     = /bin/echo
arguments      = "Hello, World!"
output         = out.$(ClusterId).$(ProcId)
error          = err.$(ClusterId).$(ProcId)
log            = log.$(ClusterId).$(ProcId)
request_cpus   = 1
request_memory = 100MB
queue
```

After you submit the job, `condor_submit` will print:

```text
Submitting job(s).
1 job(s) submitted to cluster 1.
```

Run `condor_watch_q` to watch your jobs as they transition from `IDLE` to `RUN`
to `DONE`. This may take 5 or more minutes as the pool autoscales VMs to serve
your job.

```bash
condor_watch_q
```

The output should resemble

```text
BATCH   IDLE  RUN  DONE  TOTAL  JOB_IDS
ID: 1     1    -     -      1   1.0
```

Once the pool autoscales (approx. 5 minutes), observe the output of your job:

```bash
cat out.1.0
```

You should see the output of your `echo` command: "Hello, World!"

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
./gcluster destroy htcondor-pool --auto-approve
```

When complete you should see output similar to:

```shell
Destroy complete! Resources: xx destroyed.
```

## Tutorial Complete

<walkthrough-conclusion-trophy></walkthrough-conclusion-trophy>
