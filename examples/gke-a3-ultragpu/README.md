# GKE A3 Ultra (`a3-ultragpu-8g`) Blueprint

This blueprint provisions an AI-optimized Google Kubernetes Engine (GKE) cluster with `a3-ultragpu-8g` (8x NVIDIA H200) GPU nodes, multi-NIC RoCE GPUDirect RDMA networking (`enable_dranet: true`), Kueue + JobSet workload management, NCCL gIB installer, and Cloud Storage FUSE / Managed Lustre CSI support.

## Quickstart (Deploy with `gcluster`)

### 1. Enable Required GCP APIs

```bash
gcloud services enable \
  compute.googleapis.com \
  container.googleapis.com \
  iam.googleapis.com \
  servicenetworking.googleapis.com \
  storage.googleapis.com \
  lustre.googleapis.com \
  --project=<PROJECT_ID>
```

*Note: `servicenetworking.googleapis.com` and `lustre.googleapis.com` are only required if you uncomment the optional Managed Lustre modules in `gke-a3-ultragpu.yaml`.*

### 2. Build the `gcluster` Binary

From the root of the [`cluster-toolkit`](https://github.com/GoogleCloudPlatform/cluster-toolkit) repository, build `./gcluster` (if not already built):

```bash
test -x ./gcluster || make
```

### 3. Configure `examples/gke-a3-ultragpu/gke-a3-ultragpu-deployment.yaml`

Fill in your project, region, zone, Terraform state bucket, and consumption model (Specific Reservation, DWS Flex Start, or Spot) in [`gke-a3-ultragpu-deployment.yaml`](gke-a3-ultragpu-deployment.yaml):

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME>

vars:
  deployment_name: gke-a3-ultra
  project_id: <PROJECT_ID>
  region: <REGION>
  zone: <ZONE>
  authorized_cidr: <YOUR_IP_ADDRESS>/32
  reservation_affinity:
    consume_reservation_type: SPECIFIC_RESERVATION
    specific_reservations:
    - name: <RESERVATION_NAME>
  static_node_count: <NODE_COUNT>
```

### 4. Deploy (and Destroy) the Cluster

```bash
./gcluster deploy \
  -d examples/gke-a3-ultragpu/gke-a3-ultragpu-deployment.yaml \
  examples/gke-a3-ultragpu/gke-a3-ultragpu.yaml
```

To tear down the cluster when finished:

```bash
./gcluster destroy <DEPLOYMENT_NAME>
```

## Additional Documentation

- Refer to [Deploy an A3 Ultra GKE cluster](https://cloud.google.com/cluster-toolkit/docs/deploy/gke/create-a3-ultra-cluster) and [Create an AI-optimized GKE cluster with default configuration](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#use-cluster-toolkit) for detailed instructions.
- Refer to [Deploy and run NCCL test with Topology Aware Scheduling (TAS)](https://cloud.google.com/ai-hypercomputer/docs/nccl/test-gke#a3-ultra) for instructions on running an NCCL test on the GKE A3 Ultra cluster.
