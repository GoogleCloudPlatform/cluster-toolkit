# GKE A4 (`a4-highgpu-8g`) Blueprint

This blueprint provisions an AI-optimized Google Kubernetes Engine (GKE) cluster with `a4-highgpu-8g` (8x NVIDIA B200) GPU nodes, multi-NIC RoCE GPUDirect RDMA networking (`enable_dranet: true`), Kueue + JobSet workload management, NCCL gIB installer, and Cloud Storage FUSE / Managed Lustre CSI support.

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

*Note: `servicenetworking.googleapis.com` and `lustre.googleapis.com` are only required if you uncomment the optional Managed Lustre modules in `gke-a4.yaml`.*

### 2. Build the `gcluster` Binary

From the root of the [`cluster-toolkit`](https://github.com/GoogleCloudPlatform/cluster-toolkit) repository, build `./gcluster` (if not already built):

```bash
test -x ./gcluster || make
```

### 3. Configure `examples/gke-a4/gke-a4-deployment.yaml`

Fill in your project, region, zone, Terraform state bucket, and consumption model (Specific Reservation, DWS Flex Start, or Spot) in [`gke-a4-deployment.yaml`](gke-a4-deployment.yaml):

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME>

vars:
  project_id: <PROJECT_ID>
  deployment_name: gke-a4
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
  -d examples/gke-a4/gke-a4-deployment.yaml \
  examples/gke-a4/gke-a4.yaml
```

To tear down the cluster when finished:

```bash
./gcluster destroy <DEPLOYMENT_NAME>
```

## Additional Documentation

- Refer to [Deploy an A4 GKE cluster](https://cloud.google.com/cluster-toolkit/docs/deploy/gke/create-a4-cluster) and [Create an AI-optimized GKE cluster with default configuration](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#use-cluster-toolkit) for detailed instructions.
- Refer to [Deploy and run NCCL test with Topology Aware Scheduling (TAS)](https://cloud.google.com/ai-hypercomputer/docs/nccl/test-gke#a4) for instructions on running an NCCL test on the GKE A4 cluster.
