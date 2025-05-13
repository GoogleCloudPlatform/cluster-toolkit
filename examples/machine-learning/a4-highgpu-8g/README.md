# A4 High Blueprints

## A4-High Slurm Cluster Deployment

## Set up

Replace the values for PROJECT_ID, REGION, and ZONE with the project, region,
and zone in which you have an a4-highgpu-8g allocation. The value for BUCKET
must be unique and will be used to create a new bucket. After replacing the
values, execute them so that they automatically populate parameters in the
commands shown below.

```shell
export PROJECT_ID=customer-project-id
export BUCKET=customer-bucket
export REGION=customer-region
export ZONE=customer-zone
```

## Set up a Cloud Storage bucket

To set up the storage bucket, follow the steps for your required machine type.

```shell
gcloud storage buckets create gs://${BUCKET} --project=${PROJECT_ID} \
    --default-storage-class=STANDARD --location=${REGION} \
    --uniform-bucket-level-access
gcloud storage buckets update gs://${BUCKET} --versioning
```

## Create a deployment file

Create a deployment file that you can use to specify the
Cloud Storage bucket, set names for your network and subnetwork, and set
deployment variables such as project ID, region, and zone.

To create a deployment file, follow the steps.

```yaml
terraform_backend_defaults:
type: gcs
configuration:
  bucket: BUCKET_NAME

  vars:
    deployment_name: DEPLOYMENT_NAME
    project_id: PROJECT_ID
    region: REGION
    zone: ZONE
    a4h_cluster_size: NUMBER_OF_VMS
    a4h_reservation_name: RESERVATION_NAME
```

Replace the following:

* BUCKET_NAME: the name of your Cloud Storage bucket, which you created in the
  previous section.
* DEPLOYMENT_NAME: a name for your deployment. If creating multiple clusters,
  ensure that you select a unique name for each one.
* PROJECT_ID: your project ID.
* REGION: the region that has the reserved machines.
* ZONE: the zone where you want to provision the cluster. If you're using a
  reservation-based consumption option, the region and zone information was
  provided by your Technical Account Manager (TAM) when the capacity was delivered.
* NUMBER_OF_VMS: the number of VMs that you want for the cluster.
* RESERVATION_NAME: the name of your reservation.

## Provision the Slurm cluster

To provision the cluster, run the command from the Cluster Toolkit
directory. This step takes approximately 20-30 minutes.

```shell
./gcluster deploy -d a4high-slurm-deployment.yaml examples/machine-learning/a4-highgpu-8g/a4high-slurm-blueprint.yaml --auto-approve
```

## Connect to the Slurm cluster

To access your cluster, you must login to the Slurm login node.

Use the `gcloud compute ssh`
  to connect to the login node.

```shell
  gcloud compute ssh <var>LOGIN_NODE</var> \
      --zone=<code><var>ZONE</var></code> --tunnel-through-iap
```

Replace the following:

  * `ZONE`: the zone where your VMs are created.
  * `LOGIN_NODE`: the name of the login node.

## Redeploy the Slurm cluster

If you need to increase the number of compute nodes or add new partitions to
your cluster, you might need to update configurations for your Slurm cluster by
redeploying. Redeployment can be sped up by using an existing image from a
previous deployment. To avoid creating new images during a redeploy, specify the
`--only` flag.

To redeploy the cluster using an existing image, run the command for your
required machine type.


```shell
./gcluster deploy -d a4high-slurm-deployment.yaml examples/machine-learning/a4-highgpu-8g/a4high-slurm-blueprint.yaml --only cluster-env,cluster --auto-approve -w
```

## Test network performance on the Slurm cluster

To test NCCL communication, follow the below steps.


The following test uses [Ramble](https://github.com/GoogleCloudPlatform/ramble),
which is an open-source, multi-platform experimentation framework written
in Python that is used to coordinate the running of NCCL tests.

The run scripts used for this test are staged in the
`/opt/apps/system_benchmarks` on the Slurm controller node and are
available to all nodes in the cluster. Running this test installs Ramble
to `/opt/apps/ramble`.

1. From the login node in the ${HOME} directory, run the following command.
    Because the test can take approximately 10 minutes, or longer if other
    jobs are in the queue, the following command uses `nohup` and redirects the
    `stdout/err` to a log file .

    ```shell
    nohup bash /opt/apps/system_benchmarks/run-nccl-tests-via-ramble.sh >& nccl.log &
    ```

    This command creates a folder called `nccl-tests_$(date +%s)` that stores
    all of the test results. The date tag ensures that a unique folder
    is created based on each current timestamp.

    For example, if your cluster has 16 nodes then NCCL tests are ran for
    `all-gather`, `all-reduce`, and `reduce-scatter` on 2, 4, 8, and 16 nodes.

1. Review the results. The `nccl.log` contains the logs from setting up and
    running the test. To view, you can run:

    ```shell
    tail -f nccl.log
    ```

    You can also use `Ctrl-c` to stop tailing the output at any time.
    At the end of the `nccl.log`, your output should resemble the following:

    ```bash
    ...
    ---- SUMMARY for >1GB Message Sizes ----
    workload        n_nodes msg_size        busbw
    all-gather      2       1073741824      XXX.XX
    all-gather      2       2147483648      XXX.XX
    all-gather      2       4294967296      XXX.XX
    all-gather      2       8589934592      XXX.XX
    ...
    all-reduce      2       1073741824      XXX.XX
    ...
    reduce-scatter  2       1073741824      XXX.XX
    ...
    -------- Benchmarking Complete -------
    ```

  All of the Slurm job scripts and nccl-tests output logs are
  stored in the `nccl-tests_$(date +%s)/experiments`. A summary of the
  NCCL test performance is also stored in `nccl-tests_${date +%s)/summary.tsv`.

  Removing `nccl-tests_$(date +%s)/` removes all of the files generated
  during these tests.

## Destroy the Slurm cluster

By default the A4 High blueprints enable deletion protection on the Filestore
instance. For the Filestore instance to be deleted when destroying the
Slurm cluster, follow the directions on [Set or remove deletion protection on an existing instance](/filestore/docs/deletion-protection#setting-or-removing) to disable deletion protection before
running the destroy command.

1. Before running the destroy command, navigate to the root of the Cluster Toolkit
directory. By default, `DEPLOYMENT_FOLDER` is located at the root of the
Cluster Toolkit directory.

2. To destroy the cluster, run:

```shell
./gcluster destroy DEPLOYMENT_FOLDER --auto-approve
```

Replace the following:

`DEPLOYMENT_FOLDER`: the name of the deployment folder. It's typically the same
as `DEPLOYMENT_NAME`.

When destruction is complete you should see a message similar to the following:

 ```bash
  Destroy complete! Resources: xx destroyed.
  ```

To learn how to cleanly destroy infrastructure and for advanced manual
deployment instructions, see the deployment folder located at the root of
the Cluster Toolkit directory: `DEPLOYMENT_FOLDER`/instructions.txt

## A4-High VMs

### Build the Cluster Toolkit gcluster binary

Follow instructions
[here](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment),
on how to set up your cluster toolkit environment, including enabling necessary
APIs and IAM permissions.

### (Optional, but recommended) Create a GCS Bucket for storing terraform state as mentioned above

```bash
#!/bin/bash
TF_STATE_BUCKET_NAME=<your-bucket>
PROJECT_ID=<your-gcp-project>
REGION=<your-preferred-region>

gcloud storage buckets create gs://${TF_STATE_BUCKET_NAME} \
    --project=${PROJECT_ID} \
    --default-storage-class=STANDARD --location=${REGION} \
    --uniform-bucket-level-access
gcloud storage buckets update gs://${TF_STATE_BUCKET_NAME} --versioning
```

### Obtain Filestore Zonal Capacity

We suggest using a filestore zonal instance for the best NFS performance, which
may require a quota increase request. See
[here](https://cloud.google.com/filestore/docs/requesting-quota-increases) for
more information. The Slurm and VM blueprints below default to 10TiB (10240 GiB)
instances.

### Create/modify the deployment file with your preferred configuration

For example, set the such as size, reservation to be used, etc, as well as the
name of the bucket that you just created. Below are example contents for
`a4high-vm-deployment.yaml`.

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: TF_STATE_BUCKET_NAME

vars:
  project_id: <PROJECT_ID>
  deployment_name: a4high-vm
  region: <REGION>
  zone: <ZONE>
  a4h_reservation_name: <RESERVATION_NAME>
  number_of_vms: <RESERVATION_SIZE>
```

### Additional ways to provision
Cluster toolkit also supports DWS Flex-Start, Spot VMs, as well as reservations as ways to provision instances.

[For more information on DWS Flex-Start in Slurm](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/slurm-dws-flex.md)
[For more information on Spot VMs](https://cloud.google.com/compute/docs/instances/spot)

We provide ways to enable the alternative provisioning models in the `a4high-slurm-deployment.yaml` file.

To make use of these other models, replace `a4h_reservation_name` in the deployment file with the variable of choice below.

`a4h_enable_spot_vm: true` for spot or `a4h_dws_flex_enabled: true` for DWS Flex-Start.

### Deploy the VMs

```bash
#!/bin/bash
./gcluster deploy -d a4high-vm-deployment.yaml a4high-vm.yaml --auto-approve
```
