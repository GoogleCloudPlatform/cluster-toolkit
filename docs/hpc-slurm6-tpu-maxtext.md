# Run TPU jobs in Slurm Cluster with TPU Partition

> **_NOTE:_** This tutorial will require to have TPU v4 quota. You can request
> this using [TPUv4](https://cloud.google.com/tpu/docs/quota).

This page demonstrates how to run TPU job like [maxtext](https://github.com/google/maxtext)
performance benchmark test in Slurm Cluster with TPU partition using [hpc-slurm6-tpu-maxtext.yaml](https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/main/community/examples/hpc-slurm6-tpu-maxtext.yaml)
blueprint.

## Generate the deployment and deploy the cluster

In order to deploy and run this blueprint, you need to download dataset in your
cloud storage bucket. You need follow steps mentioned in [download dataset](https://github.com/google/maxtext?tab=readme-ov-file#getting-started-download-dataset-and-configure) to download
the dataset in your GCS bucket. After that you can update the blueprint to use the
dataset from GCS bucket in training script.

```bash
./gcluster create community/examples/hpc-slurm6-tpu-maxtext.yaml --vars project_id=<project-id>;
./gcluster deploy slurm6-tpu-v4 --auto-approve
```

This would deploy slurm cluster with TPU partition, dynamic compute partition. Maxtext benchmark test script
will be stored in `/opt/apps/scripts/tpu-test` directory.

## Connect to the login node
Once the startup script has completed, connect to the login node.

Use the following command to ssh into the login node from cloud shell:

```bash
gcloud compute ssh slurm6tpuv-login-v6tpu-001 --zone us-central2-b --project <project-id>
```

You may be prompted to set up SSH. If so follow the prompts and if asked for a
password, just hit `[enter]` leaving the input blank.

## Run maxtext script

Create maxtext directory in home directory and run maxtext script.

```bash
mkdir maxtext && cd maxtext
sbatch /opt/apps/scripts/tpu-test/run_maxtext.sh
```

The sbatch command submits a batch script to the tpu partition, which makes Slurm auto-scale up nodes to run the job

You can refresh the TPU instances page and see that TPU is being/has been created.
These will be named something like `slurm6tpuv-tpunodeset-0`.

When running `squeue`, observe the job status start as `CF` (configuring), change to
`R` (running) once the compute VMs have been created, and finally `CG` (completing)
when job has finished and nodes are spooling down.

## Review the output

The `${HOME}/maxtext/output` directory will have several files and directories generated.
`slurm-<job-id>.out` file contains standard output for the TPU job.

```bash
cat slurm-1.out
```

This should have something like

```bash
completed step: 23, seconds: 4.911, TFLOP/s/device: 34.760, loss: 12.192
completed step: 24, seconds: 4.908, TFLOP/s/device: 34.781, loss: 12.173
```

This would run for the number of steps that have been provided.

## Destroy the Cluster

To avoid incurring ongoing charges we will want to destroy our cluster.

For this we need to return to our cloud shell terminal. Run exit in the terminal to close the SSH connection to the login node.

Run the following command in the cloud shell terminal to destroy the cluster:

```bash
./gcluster destroy slurm6-tpu-v4 --auto-approve
```

When complete you should see something like:

```bash
Destroy complete! Resources: xx destroyed.
```

> **_NOTE:_** If destroy is run before Slurm shut down the auto-scale nodes then
> they will be left behind and destroy may fail. In this case you can delete the
> VMs manually and rerun the destroy command above.

## Tutorial Complete
