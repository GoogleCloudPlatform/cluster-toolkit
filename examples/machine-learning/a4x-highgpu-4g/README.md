# A4X High Blueprints

This document outlines the deployment steps for provisioning A4X High
`a4x-highgpu-4g` VMs using either Slurm as an orchestrator or as a standalone VM group.

## A4X-High Slurm Cluster Deployment (`a4xhigh-slurm-blueprint.yaml`)

### 1. Enable Required GCP APIs & Build `gcluster`

Follow the instructions [here](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment) to set up your Cluster Toolkit environment, enable required APIs, and build `./gcluster` from the root of the [`cluster-toolkit`](https://github.com/GoogleCloudPlatform/cluster-toolkit) repository:

```bash
gcloud services enable \
  compute.googleapis.com \
  iam.googleapis.com \
  logging.googleapis.com \
  monitoring.googleapis.com \
  servicenetworking.googleapis.com \
  lustre.googleapis.com \
  storage.googleapis.com \
  --project=<PROJECT_ID> # logging & monitoring used by Ops Agent (enable_ops_agent: true)

test -x ./gcluster || make
```

### 2. (Optional, but recommended) Create a GCS Bucket for storing Terraform state

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

**Note:** GCS buckets created for Terraform state are not deleted by the `./gcluster destroy` command and must be deleted manually.

### 3. Managed Lustre & Cloud Storage FUSE (Slurm Blueprint)

The Slurm blueprint (`a4xhigh-slurm-blueprint.yaml`) provisions:

- **36 TiB Google Cloud Managed Lustre (`lustre_size_gib: 36000`)** mounted at `/home` across all Slurm nodes. See [Managed Lustre supported locations](https://cloud.google.com/managed-lustre/docs/locations) and [performance tiers](https://cloud.google.com/managed-lustre/docs/create-instance#performance-tiers).
- **Four Cloud Storage FUSE mounts:**
  1. `/gcs` is a general purpose mount that can be used for shared tools and data.
  2. `/gcs-checkpoints` is an optimized mount for writing and reading checkpoints. It uses the local SSD for caching and enables parallel downloads to improve performance.
  3. `/gcs-training-data` is an optimized mount for reading training data. It can be further tuned if the training data fits fully within the local SSD storage.
  4. `/gcs-model-serving` is an optimized mount for serving models, which downloads model weights in parallel to local SSD.

### 4. Modify `examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-deployment.yaml`

Set your project, region, zone, reservation, cluster size, and state bucket in `a4xhigh-slurm-deployment.yaml`:

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME>

vars:
  deployment_name: a4x-slurm
  project_id: <PROJECT_ID>
  region: <REGION>
  zone: <ZONE>
  a4x_reservation_name: <RESERVATION_NAME>
  a4x_cluster_size: <RESERVATION_SIZE>
```

### 5. Deploy the Slurm cluster

```bash
#!/bin/bash
./gcluster deploy \
  -d examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-deployment.yaml \
  examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-blueprint.yaml
```

### Re-deploy the cluster

```bash
#!/bin/bash
./gcluster deploy \
  -d examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-deployment.yaml \
  examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-blueprint.yaml \
  --only cluster-env,cluster --auto-approve -w
```

### Destroy the cluster

```bash
#!/bin/bash
./gcluster destroy <DEPLOYMENT_FOLDER> --auto-approve
```

Selective deployment and teardown for this blueprint (`cluster-env` and `cluster` groups) are documented centrally. See [examples/machine-learning/README.md](../README.md) for full details.

Example (deploy only the `cluster-env` group for this blueprint):

```bash
./gcluster deploy \
  -d examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-deployment.yaml \
  examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-blueprint.yaml \
  --only cluster-env
```

## A4X-High VM Deployment (`a4x-vm.yaml`)

This section covers deploying A4X High VMs as a standalone group, which is suitable for workloads that do not require the Slurm scheduler.

### Obtain Filestore Zonal Capacity (Standalone VM Blueprint Only)

The standalone VM blueprint (`a4x-vm.yaml`) uses a Filestore Zonal instance (`10 TiB` / `10240 GiB` default, requiring `gcloud services enable file.googleapis.com --project=<PROJECT_ID>`), which may require a quota increase request. See [Requesting Filestore quota increases](https://cloud.google.com/filestore/docs/requesting-quota-increases) for more information. *(Note: The Slurm blueprint above uses 36 TiB Managed Lustre instead of Filestore).*

### Modify the deployment file with your preferred configuration

Update `a4x-vm-deployment.yaml` with your project-specific details, such as the reservation name and desired instance count.

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME>

vars:
  deployment_name: a4x-vm
  project_id: <PROJECT_ID>
  region: <REGION>
  zone: <ZONE>
  reservation_name: <RESERVATION_NAME>
  instance_count: <INSTANCE_COUNT>
```

### Deploy the VMs

```bash
#!/bin/bash
./gcluster deploy \
  -d examples/machine-learning/a4x-highgpu-4g/a4x-vm-deployment.yaml \
  examples/machine-learning/a4x-highgpu-4g/a4x-vm.yaml
```

### Destroy the VM group

```bash
#!/bin/bash
./gcluster destroy <DEPLOYMENT_FOLDER> --auto-approve
```
