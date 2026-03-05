# A4X Max Blueprints

This document outlines the deployment steps for provisioning A4X Max `a4x-maxgpu-4g-metal` VMs using Slurm as an orchestrator.

## Blueprint Overview

This blueprint is designed to deploy a Slurm cluster on Google Cloud Platform, specifically utilizing the A4X Max machine type (`a4x-maxgpu-4g-metal`). It configures the necessary networking, storage, and software components to run jobs managed by Slurm. The setup includes bare metal nodes, advanced networking features like MRDMA, and pre-configured software stacks to leverage the underlying hardware capabilities.

Key features include:

* Deployment of `a4x-maxgpu-4g-metal` bare metal instances.
* Multi-NIC configuration including support for MRDMA over RoCE.
* Automated build of a custom image with necessary drivers and tools.
* Integration with Cloud Storage FUSE for scalable data access.
* Slurm cluster setup with controller, login, and compute nodes.
* Pre-installed NVIDIA drivers, CUDA toolkit, and DCGM for GPU monitoring.
* Installation of Mellanox OFED drivers (DOCA-OFED) for high-performance networking.

## Key Components & Versions

This blueprint installs and configures several key software components. While the blueprint aims to pull recent stable versions, specific versions installed within the image build process include:

* **Base Image Family:** `ubuntu-accelerator-2404-arm64-with-nvidia-580` (from `ubuntu-os-accelerator-images`)
* **NVIDIA CUDA Toolkit:** 13.0 (and related `datacenter-gpu-manager` packages)
* **Mellanox OFED (DOCA):** 3.2.0 for Ubuntu 24.04 arm64-sbsa
* **Mellanox Firmware Tools (MFT):** 4.34.0-145
* **Slurm:** Git ref `6.10.10` from `https://github.com/GoogleCloudPlatform/slurm-gcp`
* **NCCL Plugin Image:** `us-docker.pkg.dev/gce-ai-infra/gpudirect-gib/nccl-gib-a4x-max-arm64` (Version: `v1.1.1`)
* **ASAPd Image:** `us-docker.pkg.dev/gce-ai-infra/asapd-lite/asapd-lite:v0.0.4`

*Note: Some package managers might install minor updates to these components based on repository availability at build time.*

## Custom Image Scripts

The image build process includes several custom scripts to prepare the environment:

* **`update_gce_nic_naming.sh`**: This script updates the `gce-nic-naming` utility from the GoogleCloudPlatform/guest-configs repository. This is often necessary to ensure correct and consistent network interface naming (e.g., eth0, eth1) across different GCE instance types or image versions, which is crucial for multi-NIC setups like the one used in A4X Max.
* **`apply_networkd_workaround.sh`**: This script applies a workaround for `systemd-networkd-wait-online`. On some systems, particularly newer Ubuntu versions, this service can cause significant delays during boot waiting for all links to become fully "routable". The override adjusts the command to wait for *any* interface to be operational, speeding up the boot process on nodes with multiple network interfaces, some of which might be configured differently (e.g., MRDMA interfaces).
* **`setup_asapd_lite_service.sh`**: Configures and starts the `asapd-lite` service, which runs in a Docker container. This service is part of the networking stack required for A4X Max functionality.
* **`install_mofed.sh`**: Handles the download and installation of the Mellanox OpenFabrics Enterprise Distribution (OFED) drivers, specifically the DOCA variant, required for the RDMA network interfaces.
* **`enable_openibd.sh`**: Ensures the `openibd` service, part of the OFED stack, is enabled to start on boot.

## A4X-Max Slurm Cluster Deployment

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

## A4X-Max Slurm Cluster Deployment

### Create/modify the deployment file with your preferred configuration

For example, set the such as size, reservation to be used, etc, as well as the
name of the bucket that you just created. Below are example contents for `a4xmax-slurm-deployment.yaml`.

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME> # Replace with your bucket name

vars:
  deployment_name: a4x-max-slurm
  project_id: <PROJECT_ID> # Replace with your GCP project ID
  region: <REGION> # Replace with your desired region
  zone: <ZONE> # Replace with your desired zone
  a4x_max_reservation_name: <RESERVATION_NAME> # Optional: Replace with your reservation name
  a4x_max_cluster_size: <RESERVATION_SIZE> # Replace with your desired cluster size

```

### Deploy the cluster

``` bash
#!/bin/bash
./gcluster deploy -d examples/machine-learning/a4x-maxgpu-4g-metal/a4xmax-bm-slurm-deployment.yaml examples/machine-learning/a4x-maxgpu-4g-metal/a4xmax-bm-slurm-blueprint.yaml
```

### Re-deploy the cluster

```bash
#!/bin/bash
./gcluster deploy -d examples/machine-learning/a4x-maxgpu-4g-metal/a4xmax-bm-slurm-deployment.yaml examples/machine-learning/a4x-maxgpu-4g-metal/a4xmax-bm-slurm-blueprint.yaml --only cluster-env,cluster --auto-approve -w
```

### Destroy the cluster

```bash
#!/bin/bash
./gcluster destroy <DEPLOYMENT_FOLDER> --auto-approve
```

Selective deployment and teardown for this blueprint are documented centrally. See [examples/machine-learning/README.md](../README.md) for full details.

Example (deploy only the primary group for this blueprint):

```bash
./gcluster deploy -d examples/machine-learning/a4x-maxgpu-4g-metal/a4xmax-bm-slurm-deployment.yaml examples/machine-learning/a4x-maxgpu-4g-metal/a4xmax-bm-slurm-blueprint.yaml --only primary
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
