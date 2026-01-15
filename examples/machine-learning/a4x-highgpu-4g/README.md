# A4X High Blueprints

This document outlines the deployment steps for provisioning A4X High
`a4x-highgpu-4g` VMs using Slurm as an orchestrator.

## A4X-High Slurm Cluster Deployment

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

## A4X-High Slurm Cluster Deployment

### Create/modify the deployment file with your preferred configuration

For example, set the such as size, reservation to be used, etc, as well as the
name of the bucket that you just created. Below are example contents for `a4xhigh-slurm-deployment.yaml`.

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: TF_STATE_BUCKET_NAME

vars:
  deployment_name: a4x-slurm
  project_id: <PROJECT_ID>
  region: <REGION>
  zone: <ZONE>
  a4x_reservation_name: <RESERVATION_NAME>
  a4x_cluster_size: <RESERVATION_SIZE>
```

### Deploy the cluster

``` bash
#!/bin/bash
gcluster deploy -d a4xhigh-slurm-deployment.yaml a4xhigh-slurm-blueprint.yaml
```

### Re-deploy the cluster

```bash
#!/bin/bash
gcluster deploy -d a4xhigh-slurm-deployment.yaml examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-blueprint.yaml --only cluster-env,cluster --auto-approve -w
```

### Destroy the cluster

```bash
#!/bin/bash
./gcluster destroy <DEPLOYMENT_FOLDER> --auto-approve
```

Selective deployment and teardown for this blueprint are documented centrally. See [examples/machine-learning/README.md](../README.md) for full details.

Example (deploy only the primary group for this blueprint):

```bash
./gcluster deploy -d a4xhigh-slurm-deployment.yaml a4xhigh-slurm-blueprint.yaml --only primary
```

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
