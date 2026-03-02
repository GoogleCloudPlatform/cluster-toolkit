# TPU v6e DWS Queued Provisioning

This example demonstrates how to deploy a GKE cluster with **TPU v6e** nodes using **Dynamic Workload Scheduler (DWS)** with **Queued Provisioning**.

## Overview

This configuration sets up:

* A GKE cluster with a dedicated TPU v6e node pool (`ct6e-standard-4t`).
* **Flex Start (Dynamic Scaling)**: The node pool scales from 0 to N nodes based on demand.
* **Queued Provisioning**: Jobs are queued until the entire requested capacity is available, ensuring "all-or-nothing" scheduling.
* **Kueue Orchestration**: Manages the job queue and provisioning requests.

## Create a cluster

These steps guide you through the cluster creation process.

Note: If you create multiple clusters using these same cluster blueprints, ensure that all VPCs and subnet names are unique per project to prevent errors.

1. Launch [Cloud Shell](https://cloud.google.com/shell/docs/launching-cloud-shell). You can use a different environment; however, we recommend Cloud Shell because the dependencies are already pre-installed for Cluster Toolkit. If you don't want to use Cloud Shell, follow the [instructions to install dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) to prepare a different environment.
1. Clone the Cluster Toolkit from the git repository:

   ```sh
   cd ~
   git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
   ```

1. Install the Cluster Toolkit:

   ```sh
   cd cluster-toolkit && git checkout main && make
   ```

1. Create a Cloud Storage bucket to store the state of the Terraform deployment:

   ```sh
   gcloud storage buckets create gs://BUCKET_NAME \
       --project=PROJECT_ID \
       --default-storage-class=STANDARD \
       --location=COMPUTE_REGION \
       --uniform-bucket-level-access
   gcloud storage buckets update gs://BUCKET_NAME --versioning
   ```

   Replace the following variables:

   * BUCKET_NAME: the name of the new Cloud Storage bucket.
   * PROJECT_ID: ID of the project where the bucket is being created.
   * COMPUTE_REGION: the compute region where you want to store the state of the Terraform deployment.

1. In the `examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-tpu-v6e/gke-tpu-v6e-deployment.yaml` file, fill in the following settings in the `terraform_backend_defaults` and `vars` sections to match the specific values for your deployment:

   * `bucket`: the name of the Cloud Storage bucket you created in the previous step.
   * `project_id`: your Google Cloud project ID.
   * `deployment_name`: a unique name for this deployment.
   * `region`: the compute region for the cluster.
   * `zone`: the compute zone for the node pool.
   * `authorized_cidr`: The IP address range that you want to allow to connect with the cluster (e.g., `0.0.0.0/0`).
   * **`tpu_topology`**: Defaults to `4x4` (16 chips).
   * **`autoscaling_max_node_count`**: **Must match your topology.** For a `4x4` (16 chips) topology using 4-chip nodes, this must be set to `4` (16 / 4 = 4).
   * **`autoscaling_min_node_count`**: Must be `0`.
   * **`enable_flex_start`**: Must be `true`.
   * **`enable_queued_provisioning`**: Must be `true`.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

   ```sh
   gcloud auth application-default login
   ```

1. Deploy the blueprint to provision the GKE infrastructure using TPU v6e machine types:

   ```sh
   cd ~/cluster-toolkit
   ./gcluster deploy -d \
   examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-tpu-v6e/gke-tpu-v6e-deployment.yaml \
   examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-tpu-v6e/gke-tpu-v6e.yaml
   ```

1. When prompted, select (A)pply to deploy the blueprint.
   * The blueprint creates VPC networks, Cloud Storage buckets, service accounts, a GKE cluster with a TPU node pool, Kueue, and JobSet.

1. Get Credentials:

   ```bash
   gcloud container clusters get-credentials <cluster-name> --region <region> --project <project-id>
   ```

## Running Jobs

Two sample JobSets are provided:
* `tpu-v6e-test-job.yaml`: A simple JobSet that echoes a message and sleeps. Best for initial cluster verification.
* `tpu-v6e-test-job-gcs.yaml`: A JobSet that performs an **FIO benchmark** against your provisioned GCS buckets (training/checkpointing). This validates:
  * **Workload Identity**: Ensures pods can authenticate to Google Cloud APIs.
  * **GCS Fuse CSI**: Verifies that GCS buckets are correctly mounted as local filesystems.
  * **Storage Performance**: Measures read/write throughput to the buckets.

### Option 1: Simple Test

#### Submit Job

```bash
kubectl apply -f examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-tpu-v6e/tpu-v6e-test-job.yaml
```

### Option 2: GCS Storage Benchmark (FIO)

#### Find your PVC Names

The toolkit creates dynamic names for your GCS buckets. Find them with:

```bash
kubectl get pvc
```

#### Update Manifest

Edit `tpu-v6e-test-job-gcs.yaml` and replace the `claimName` placeholders (e.g., `REPLACE_WITH_YOUR_TRAINING_PVC`) with your actual PVC names from the previous step.

#### Submit Job

```bash
kubectl apply -f examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-tpu-v6e/tpu-v6e-test-job-gcs.yaml
```

## Monitor Provisioning

Check the status of the DWS request:

```bash
kubectl get provisioningrequests -w
```

* `ACCEPTED`: Request is queued.
* `PROVISIONED`: Resources are allocated, nodes are creating.

1. Verify Execution:
   Once nodes are ready, the pods will start:

   ```bash
   kubectl get pods -w
   ```

## Verifying Scale-Up and Scale-Down

To ensure the cluster is behaving correctly, you can monitor the following events:

### 1. Monitor Scale-Up

When the job is submitted, the cluster will scale from 0 nodes to the required count (e.g., 4 nodes).
* Watch Nodes: `kubectl get nodes -w`
* Check Autoscaler Status:

   ```bash
   kubectl get configmap cluster-autoscaler-status -n kube-system -o yaml
   ```

   Look for `scaleUp: status: NoActivity` transitioning to activity and `ready` node counts increasing.

### 2. Verify Job Success

A successful DWS run means the job started *after* the full slice was provisioned and completed its work.
* Check Pod Status: `kubectl get pods` should show `STATUS: Completed`.
* Check Logs: `kubectl logs -l job-name=tpu-v6e-qp-test` should show the "Job complete!" message.

### 3. Monitor Scale-Down

After the job completes, the Cluster Autoscaler will wait for a short period (typically 10 minutes) before deleting the unneeded TPU nodes.
* Observe Node Deletion: `kubectl get nodes -w` will eventually show nodes being removed.
* Confirm Zero State: `kubectl get nodes` should eventually return to only showing your system nodes.

## Custom Jobs Requirements

If you want to submit your own custom job, ensure the following fields are included in your manifest:

### 1. Metadata (Kueue & DWS)

Required for the job to be admitted to the queue and recognized by DWS.
Note: The `queue-name` must match the `LocalQueue` created by the toolkit (default: `dws-local-queue`).

```yaml
metadata:
  labels:
    kueue.x-k8s.io/queue-name: dws-local-queue
  annotations:
    provreq.kueue.x-k8s.io/maxRunDurationSeconds: "3600" # Specify duration in seconds
```

### 2. Node Selectors & Affinity

Ensures the job lands on the specific provisioned TPU nodes:

```yaml
nodeSelector:
  cloud.google.com/gke-tpu-topology: "4x4"
  cloud.google.com/gke-queued: "true"
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
      - matchExpressions:
        - key: cloud.google.com/gke-nodepool
          operator: In
          values: ["gke-tpu-v6e-pool"]
```

### 3. Tolerations (Mandatory)

Required to allow pods to land on tainted TPU nodes:

```yaml
tolerations:
- key: "google.com/tpu"
  operator: "Equal"
  value: "present"
  effect: "NoSchedule"
- key: "cloud.google.com/gke-queued"
  operator: "Equal"
  value: "true"
  effect: "NoSchedule"
```

## Validation

### 1. Simple Test Validation

If you ran `tpu-v6e-test-job.yaml`, check logs for the success message:

```bash
kubectl logs -l jobset.sigs.k8s.io/jobset-name=tpu-v6e-qp-test -c tpu-job
```

Expected output:

```text
Starting TPU v6e Test Job...
Job complete!
```

### 2. GCS Storage Benchmark Validation

If you ran `tpu-v6e-test-job-gcs.yaml`, you can verify the benchmark results and storage health:

1. **Verify Completion**: Look for the final success message in the logs:

   ```bash
   kubectl logs -l jobset.sigs.k8s.io/jobset-name=tpu-v6e-qp-fio -c tpu-job | grep "FIO benchmark complete!"
   ```

1. **View Performance Metrics**: To see the actual read/write throughput for your GCS buckets:

   ```bash
   kubectl logs -l jobset.sigs.k8s.io/jobset-name=tpu-v6e-qp-fio -c tpu-job
   ```

   In the output, look for the `Run status group` sections. For example:
   * **Read Performance**: Look for `READ: bw=...` (e.g., `bw=5554MiB/s`).
   * **Write Performance**: Look for `WRITE: bw=...`.

> [!TIP]
> If the job is still running, you can follow the logs in real-time by adding the `-f` flag to the `kubectl logs` command.
