# A4 High Blueprints

## A4-High Slurm Cluster Deployment
For further information on deploying an A4 High cluster with Slurm, please
see:

[Create an AI-optimized Slurm cluster](https://cloud.google.com/ai-hypercomputer/docs/create/create-slurm-cluster)

### Selective Deployment and Destruction using --only and --skip flags

You can control which parts of a blueprint are deployed or destroyed using the `--only` and `--skip` flags with the `gcluster deploy` and `gcluster destroy` commands. This is useful for saving time by not acting on components unnecessarily or for more granular control over resources.

A4-High blueprint is divided into logical groups. Common groups include `cluster-env`, `cluster`, `image-env`, and `image`. The exact groups available depend on the blueprint definition.

#### `--only <group1>,<group2>,...`

Use the `--only` flag to have the command act on *only* the specified, comma-separated groups. Other groups will be untouched.

**Examples:**

* Deploy only the `cluster-env` group:

    ```bash
    ./gcluster deploy -d a4high-slurm-deployment.yaml examples/machine-learning/a4-highgpu-8g/a4high-slurm-blueprint.yaml --only cluster-env
    ```

* Destroy only the `image` group:

    ```bash
    ./gcluster destroy deployment-name --only image
    ```

* Deploy only the `cluster-env` and `cluster` groups:

    ```bash
    ./gcluster deploy -d a4high-slurm-deployment.yaml examples/machine-learning/a4-highgpu-8g/a4high-slurm-blueprint.yaml --only cluster-env,cluster
    ```

#### `--skip <group1>,<group2>,...`

Use the `--skip` flag to have the command act on all groups *except* those specified in the comma-separated list.

**Examples:**

* Deploy everything *except* the `image` group:

    ```bash
    ./gcluster deploy -d a4high-slurm-deployment.yaml examples/machine-learning/a4-highgpu-8g/a4high-slurm-blueprint.yaml --skip image
    ```

* Destroy everything *except* the `cluster-env` group:

    ```bash
    ./gcluster destroy deployment-name --skip cluster-env
    ```

**Use Cases:**

* **Faster Iteration:** When developing, only `deploy` the group you are modifying (e.g., `--only cluster-env`).
* **Partial Teardown:** Selectively `destroy` parts of a deployment without affecting others (e.g., `--only image` to remove image but keep networking and other things).
* **Avoiding Unchanged Parts:** Use `--skip` to not redeploy or destroy parts you know are stable or should be preserved (e.g., `--skip cluster,image`).
* **Retry Failed Operations:** If a `deploy` or `destroy` fails on a specific group, you can rerun the command targeting just that group using `--only`.

### Cloud Storage FUSE

This blueprint includes four Cloud Storage FUSE mounts to provide a simple and scalable way
to manage data.

1. `/gcs` is a general purpose mount that can be used for shared tools and data.
1. `/gcs-checkpoints` is an optimized mount for writing and reading checkpoints. It
    uses the local SSD for caching and enables parallel downloads to improve
    performance.
1. `/gcs-training-data` is an optimized mount for reading training data. It can
   be further tuned if the training data fits fully within the local ssd
   storage.
1. `/gcs-model-serving` is an optimized mount for serving models, which
   downloads model weights in parallel to local ssd.

## A4-High VMs

### Build the Cluster Toolkit gcluster binary

Follow instructions
[here](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment),
on how to set up your cluster toolkit environment, including enabling necessary
APIs and IAM permissions.

### (Optional, but recommended) Create a GCS Bucket for storing terraform state

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

We provide ways to enable the alternative provisioning models in the
`a4high-slurm-deployment.yaml` file.

To make use of these other models, replace `a4h_reservation_name` in the
deployment file with the variable of choice below.

`a4h_enable_spot_vm: true` for spot or `a4h_dws_flex_enabled: true` for DWS Flex-Start.

### Deploy the VMs

```bash
#!/bin/bash
./gcluster deploy -d a4high-vm-deployment.yaml a4high-vm.yaml --auto-approve
```
