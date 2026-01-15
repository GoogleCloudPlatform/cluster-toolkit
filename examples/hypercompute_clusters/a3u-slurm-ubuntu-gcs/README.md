>[!WARNING]
>
>![deprecated-badge] This blueprint is not maintained any longer. The blueprint will be removed by Feb 15, 2026
>
>We recommend using the [A3 Ultra Slurm Blueprint](../../machine-learning/a3-ultragpu-8g) instead.

# A3-Ultra Slurm + Ubuntu + GCS

This reference design creates a Slurm cluster with the following design:

1. Ubuntu 22 Operating System
1. A static a3-ultragpu-8g partition that uses a reservation.
1. 3 VPCs (2x CPU, 1x for GPU RDMA networks), with a total of 9 subnetworks
1. A GCS bucket that is configured with Hierarchical Namespace enabled
1. Cloud Storage Fuse, configured to utilize Local-SSD storage

## Deployment Instructions

### Build the Cluster Toolkit gcluster binary

Follow instructions
[here](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment)

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

### Create and configure a GCS Bucket

This will be used for input data and checkpoint/restart data. This bucket should
be created with Hierarchical Namespace enabled. See
[here](https://cloud.google.com/storage/docs/hns-overview) for more details.

```bash
#!/bin/bash
PROJECT_ID=<your-gcp-project>
REGION=<your-preferred-region>
HNS_BUCKET_NAME=<training-bucket-name>
PROJECT_NUMER=<your-project-number>

gcloud storage buckets create gs://${HNS_BUCKET_NAME} \
    --location=${REGION} --uniform-bucket-level-access
    --enable-hierarchical-namespace

```

### Create/modify the deployment.yaml file with your preferred configuration

For example, set the such as size, reservation to be used, etc, as well as the
name of the bucket that you just created. Below is an example

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: TF_STATE_BUCKET_NAME

vars:
  deployment_name: a3u-gcs
  project_id: <PROJECT_ID>
  region: <REGION>
  zone: <ZONE>
  a3u_reservation_name: <RESERVATION_NAME>
  a3u_cluster_size: <RESERVATION_SIZE>
  hns_gcs_bucket: <HNS_BUCKET_NAME> # This bucket must have been previously created

```

### Deploy the cluster

```bash
#!/bin/bash
gcluster deploy -d deployment.yaml a3u-slurm-ubuntu-gcs.yaml
```

## Storage Design Components

On the login and controller nodes, the gcs bucket is mounted at /gcs, using
fairly standard [Cloud Storage Fuse configuration](https://cloud.google.com/storage/docs/cloud-storage-fuse/config-file). On the compute nodes, there are two
mounts of the same bucket.  First, `/gcs` is mounted with with the following
configuration:

```yaml
file-cache:
  max-size-mb: -1
  enable-parallel-downloads: true
  download-chunk-size-mb: 50
  parallel-downloads-per-file: 16
cache-dir: /mnt/localssd
file-system:
  dir-mode: "777"
  file-mode: "777"
  rename-dir-limit: 20000  # Set to 20000 for hierarchical buckets
  temp-dir: /mnt/localssd
  fuse-options: allow_other
foreground: true
```

This uses /mnt/localssd as a cache dir (for reads) and temp-dir (for writes).
It also enables parallel downloads, which is particularly useful for
checkpoint restarts.

Next, `/gcs-ro` is mounted in a "read-only" mode, and optimized to for
input (training) data reading.

```yaml
file-cache:
  max-size-mb: -1
metadata-cache:
  ttl-secs: 3600  # Decrease if your data changes quickly.
cache-dir: /mnt/localssd
file-system:
  dir-mode: "755" # need 5 on dir to enable ls
  file-mode: "644"
  temp-dir: /mnt/localssd
  fuse-options: allow_other
  kernel-list-cache-ttl-secs: 60
foreground: true
```

The local ssds will be used for a file cache, and the metadata-cache
for the data is set to 1 hour, with kernel-list-cache ttl set to 60 seconds.
This reduces the amount of requests that will be sent to GCS, and improves
data loading performance.

We suggest using /gcs for checkpoint saving/loading. and use /gcs-ro for
data input loading.

## Running Benchmarks with Ramble

See the subdirectory `system_benchamrks`, where you will find instructions
on how to run NCCL, HPL, and NeMo benchmarks via
[ramble](https://github.com/GoogleCloudPlatform/ramble).

[deprecated-badge]: https://img.shields.io/badge/-deprecated-%23fea2a2?style=plastic
