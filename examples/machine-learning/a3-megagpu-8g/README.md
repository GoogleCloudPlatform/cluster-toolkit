# A3 Mega (`a3-megagpu-8g`) Slurm Blueprint for Google Cloud

This directory provides a Cluster Toolkit blueprint ([`a3mega-slurm-blueprint.yaml`](a3mega-slurm-blueprint.yaml)) and deployment configuration ([`a3mega-slurm-deployment.yaml`](a3mega-slurm-deployment.yaml)) for provisioning a Slurm cluster with `a3-megagpu-8g` (8x NVIDIA H100) compute nodes, GPUDirect-TCPXO multi-NIC networking (8 GPU VPCs), and a **36 TiB Google Cloud Managed Lustre** filesystem (`lustre_size_gib: 36000`) mounted at `/home`.

For additional details, see [Deploy an A3 Mega Slurm cluster](https://cloud.google.com/cluster-toolkit/docs/deploy/deploy-a3-mega-cluster). Selective deployment and teardown across the 4 deployment groups (`primary`, `build_script`, `slurm-build`, and `cluster`) are documented centrally in [examples/machine-learning/README.md](../README.md).

## Quickstart (`a3mega-slurm-blueprint.yaml`)

### 1. Enable Required GCP APIs

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
```

### 2. Build the `gcluster` Binary

From the root of the [`cluster-toolkit`](https://github.com/GoogleCloudPlatform/cluster-toolkit) repository, build `./gcluster` (if not already built):

```bash
test -x ./gcluster || make
```

### 3. Configure `examples/machine-learning/a3-megagpu-8g/a3mega-slurm-deployment.yaml`

Fill in your Terraform state bucket, project, region, zone, VPC/subnetwork names, Slurm cluster name, cluster size, and consumption model in [`a3mega-slurm-deployment.yaml`](a3mega-slurm-deployment.yaml):

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME>

vars:
  deployment_name: a3mega-slurm
  project_id: <PROJECT_ID>
  region: <REGION>
  zone: <ZONE>
  network_name_system: a3mega-sys-net
  subnetwork_name_system: a3mega-sys-subnet
  enable_ops_agent: true
  enable_nvidia_dcgm: true
  enable_nvidia_persistenced: true
  final_image_family: slurm-a3mega
  slurm_cluster_name: a3megaslurm
  a3mega_cluster_size: <CLUSTER_SIZE>
  a3mega_reservation_name: <RESERVATION_NAME>
  # Additional provisioning models (pick only one), can be used to substitute `a3mega_reservation_name`:
  a3mega_dws_flex_enabled: false
  a3mega_enable_spot_vm: false
```

### 4. Deploy (and Destroy) the Slurm Cluster

Deploy all 4 deployment groups (`primary`, `build_script`, `slurm-build`, `cluster`):

```bash
./gcluster deploy \
  -d examples/machine-learning/a3-megagpu-8g/a3mega-slurm-deployment.yaml \
  examples/machine-learning/a3-megagpu-8g/a3mega-slurm-blueprint.yaml
```

Example (deploy only the `primary` group for this blueprint):

```bash
./gcluster deploy \
  -d examples/machine-learning/a3-megagpu-8g/a3mega-slurm-deployment.yaml \
  examples/machine-learning/a3-megagpu-8g/a3mega-slurm-blueprint.yaml \
  --only primary
```

To tear down the cluster when finished:

```bash
./gcluster destroy <DEPLOYMENT_NAME>
```

## Managed Lustre & GCSFuse with Local SSD Cache

- **Managed Lustre (`a3mega-slurm-blueprint.yaml`):** Provisions a **36 TiB Google Cloud Managed Lustre (`lustre_size_gib: 36000`, `per_unit_storage_throughput: 500`)** instance mounted at `/home` across all Slurm nodes.
- **GCSFuse with Local SSD Cache (`a3mega-slurm-gcsfuse-lssd-blueprint.yaml`):** Reflects best practices for using GCSFuse for ML workloads. It is configured to mount GCS buckets on two mountpoints on a3-mega nodes. Use the `gcs_bucket` variable to specify a GCS bucket to mount, or leave the variable empty to mount all available buckets [dynamically](https://cloud.google.com/storage/docs/cloud-storage-fuse/mount-bucket#dynamic-mount).
  The `/gcs` mountpoint enables parallel downloads, intended for reading/writing checkpoints, logs, application outputs, model serving, or loading large files (e.g. squashfs files). The read-only `/gcs-ro` mountpoint disables parallel downloads and enables the list cache, intended for reading training data. Parallel downloads are not recommended for training workloads; see [GCSFuse documentation](https://cloud.google.com/storage/docs/cloud-storage-fuse/file-caching#parallel-downloads) for details.
