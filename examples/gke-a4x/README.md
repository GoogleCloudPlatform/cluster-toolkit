# GKE A4X (`a4x-highgpu-4g`) Blueprint

This blueprint provisions an AI-optimized Google Kubernetes Engine (GKE) cluster with `a4x-highgpu-4g` (4x NVIDIA GB200) GPU nodes, multi-NIC RoCE GPUDirect RDMA networking (`enable_dranet: true`), Kueue + JobSet workload management, NCCL gIB installer, and Cloud Storage FUSE / Managed Lustre CSI support.

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

*Note: `servicenetworking.googleapis.com` and `lustre.googleapis.com` are only required if you uncomment the optional Managed Lustre modules in `gke-a4x.yaml`.*

### 2. Build the `gcluster` Binary

From the root of the [`cluster-toolkit`](https://github.com/GoogleCloudPlatform/cluster-toolkit) repository, build `./gcluster` (if not already built):

```bash
test -x ./gcluster || make
```

### 3. Configure `examples/gke-a4x/gke-a4x-deployment.yaml`

Fill in your project, region, zone, Terraform state bucket, reservation, and node counts in [`gke-a4x-deployment.yaml`](gke-a4x-deployment.yaml):

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME>

vars:
  project_id: <PROJECT_ID>
  deployment_name: gke-a4x
  region: <REGION>
  zone: <ZONE>
  static_node_count: <NODE_COUNT>
  authorized_cidr: <YOUR_IP_ADDRESS>/32
  reservation: <RESERVATION_NAME>
  reservation_project_id: <RESERVATION_PROJECT_ID>
  num_node_pools: 1
```

### 4. Deploy (and Destroy) the Cluster

```bash
./gcluster deploy \
  -d examples/gke-a4x/gke-a4x-deployment.yaml \
  examples/gke-a4x/gke-a4x.yaml
```

To tear down the cluster when finished:

```bash
./gcluster destroy <DEPLOYMENT_NAME>
```

## Additional Documentation

- Refer to [Deploy an A4X GKE cluster](https://cloud.google.com/cluster-toolkit/docs/deploy/gke/create-a4x-cluster) and [Create an AI-optimized GKE cluster with default configuration](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#use-cluster-toolkit) for detailed instructions.
- Refer to [Deploy and run NCCL test with Topology Aware Scheduling (TAS)](https://cloud.google.com/ai-hypercomputer/docs/nccl/test-gke#a4x) for instructions on running an NCCL test on the GKE-A4X cluster.

## Install and Run MPI Operator on GKE Cluster

The Kubeflow MPI Operator manages distributed MPI workloads on GKE.

1. **Deploy MPI Operator (v0.8.2):**

   - **Automated (During Cluster Creation via Blueprint YAML):** Include the MPI Operator manifest in `apply_manifests` under `kubectl-apply` in your blueprint YAML (`gke-a4x.yaml`):

     ```yaml
       - id: kubectl-apply
         source: modules/management/kubectl-apply
         use: [a4x-cluster]
         settings:
           apply_manifests:
           - name: mpi-operator
             source: https://raw.githubusercontent.com/kubeflow/mpi-operator/v0.8.2/deploy/v2beta1/mpi-operator.yaml
     ```

   - **Manual (After Cluster Deployment via `kubectl`):** Once the cluster is deployed, run the following command against your cluster:

     ```bash
     kubectl apply --server-side -f https://raw.githubusercontent.com/kubeflow/mpi-operator/v0.8.2/deploy/v2beta1/mpi-operator.yaml
     ```

2. **Verify Installation:**

   ```bash
   kubectl get crd | grep mpijob
   kubectl get pods -n mpi-operator
   ```

3. **Run a Sample MPIJob Test:**
   Create a test manifest `sample-mpijob.yaml`:

   ```yaml
   apiVersion: kubeflow.org/v2beta1
   kind: MPIJob
   metadata:
     name: sample-mpi-job
     namespace: default
   spec:
     slotsPerWorker: 1
     runPolicy:
       cleanPodPolicy: Running
     mpiReplicaSpecs:
       Launcher:
         replicas: 1
         template:
           spec:
             containers:
             - name: mpi-launcher
               image: mpioperator/mpi-pi:v0.8.2-openmpi
               command:
               - mpirun
               - --allow-run-as-root
               - -n
               - "2"
               - --hostfile
               - /etc/mpi/hostfile
               - echo
               - "Hello World from MPI worker!"
       Worker:
         replicas: 2
         template:
           spec:
             containers:
             - name: mpi-worker
               image: mpioperator/mpi-pi:v0.8.2-openmpi
   ```

   Submit the job and inspect launcher logs:

   ```bash
   kubectl apply -f sample-mpijob.yaml
   kubectl logs -l training.kubeflow.org/job-role=launcher
   kubectl delete -f sample-mpijob.yaml
   ```
