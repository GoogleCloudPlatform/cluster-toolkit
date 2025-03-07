# A4 High Blueprints

This document outlines the deployment steps for provisioning A4 High
`a4-highgpu-8g` VMs both with and without using Slurm as an orchestrator.

## Shared Instructions

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

## A4-High Slurm Cluster Deployment

### Create/modify the deployment file with your preferred configuration

For example, set the such as size, reservation to be used, etc, as well as the
name of the bucket that you just created. Below are example contents for
`a4high-slurm-deployment.yaml`.

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: TF_STATE_BUCKET_NAME

vars:
  deployment_name: a4h-slurm
  project_id: <PROJECT_ID>
  region: <REGION>
  zone: <ZONE>
  a4h_reservation_name: <RESERVATION_NAME>
  a4h_cluster_size: <RESERVATION_SIZE>
```

### Deploy the cluster

```bash
#!/bin/bash
gcluster deploy -d a4high-slurm-deployment.yaml a4high-slurm-blueprint.yaml
```

## A4-High VMs

### Create/modify the deployment file with your preferred configuration

For example, set the such as size, reservation to be used, etc, as well as the
name of the terraform state bucket from the "Shared Instructions" above. Below
are example contents for `a4high-vm-deployment.yaml`.

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME>

vars:
  deployment_name: a4high-vm
  project_id: <PROJECT_ID> # supply project ID
  region: <REGION>
  zone: <ZONE>
  a4h_reservation_name: <RESERVATION_NAME> # supply reservation name
  number_of_vms: <RESERVATION_SIZE>
```

### Deploy the VMs

```bash
#!/bin/bash
gcluster deploy -d a4high-vm-deployment.yaml a4high-vm.yaml
```
