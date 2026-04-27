# ML Diagnostics Sample Workload Test

This guide explains how to build and run a sample ML workload to test the Google Cloud ML Diagnostics setup on your GKE TPU cluster. The sample job utilizes a custom container image to ensure all dependencies are self-contained.

For more background, refer to the [ML Diagnostics SDK documentation](https://docs.cloud.google.com/tpu/docs/ml-diagnostics/sdk). While that documentation describes manual execution within a cluster, this guide uses a `Dockerfile` to package the application and its dependencies, simplifying deployment on GKE. The Python script within the Docker image is configured to use environment variables for project ID, region, and GCS path.

## Prerequisites

To run a sample job, you need a GKE TPU cluster with ML Diagnostics enabled:

1. Deploy a GKE TPU blueprint (e.g., v6e or 7x) with ML Diagnostics features enabled. Instructions can be found in the respective READMEs:

   - [GKE TPU v6e README](../../examples/gke-tpu-v6e/README.md#understanding-ml-diagnostics-integration)
   - [GKE TPU 7x README](../../examples/gke-tpu-7x/README.md#understanding-ml-diagnostics-integration)

2. Verify the cluster and ML Diagnostics components are correctly configured by following the "Testing ML Diagnostics Cluster Configuration" section in the blueprint's README.

Refer to the [ML Diagnostics documentation overview](https://docs.cloud.google.com/tpu/docs/ml-diagnostics/overview) for more general information.

## Steps to Run Sample Workload

### 1. Create a Docker repository in Artifact Registry

Navigate to the test folder within your cloned `cluster-toolkit` repository:

```sh
cd modules/management/diagnostics/sample-workload-test
```

Set environment variables for your project and region:

```sh
export REGION=<region> # Replace with the region used in your blueprint (e.g., us-central1)
export PROJECT_ID=<project_id> # Replace with your Google Cloud Project ID
export REPO_NAME=mldiagnostics-tests # Or your preferred repository name
```

Create an Artifact Registry repository:

```sh
gcloud artifacts repositories create ${REPO_NAME} \
    --repository-format=docker \
    --project="${PROJECT_ID}" \
    --location="${REGION}"
```

### 2. Build and Push Docker Image

Set the image URI (replace `<image-name>` with your choice, e.g., sample-workload):

```sh
export IMAGE_URI="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}/<image-name>:latest"
```

Build and push the image using the provided Dockerfile:

```sh
docker build -t "${IMAGE_URI}" .
docker push "${IMAGE_URI}"
```

### 3. Modify `sample_job.yaml`

Open the `sample_job.yaml` file and update the placeholder values to match your environment. The lines requiring changes are indicated below:

- `namespace: <user_namespace>` (Line 19 & 29): Replace <user_namespace> with the workload namespace defined in your blueprint deployment (e.g., ai-workloads).
- `serviceAccountName: <k8s-service-account-name>` (Line 48): Replace <k8s-service-account-name> with the Kubernetes service account name configured for Workload Identity in your blueprint (default is workload-identity-k8s-sa).
- `nodeSelector` (Lines 52-53):
  - cloud.google.com/gke-tpu-accelerator: <tpu_accelerator>: Replace <tpu_accelerator> with the correct value for your TPU version (e.g., tpu-v6e-slice for v6e, tpu7x for 7x).
  - cloud.google.com/gke-tpu-topology: <tpu_topology>: Replace <tpu_topology> with the topology used in your blueprint (e.g., 2x2x1, 4x4).
- `image: <workload_image>` (Line 57): Replace <workload_image> with the IMAGE_URI from Step 2.
- `env variables` (Lines 60-64): These are passed to the container and used by the Python script.
  - `PROJECT_ID:` Replace `<project_id>` with your Project ID.
  - `REGION:` Replace `<region>` with your cluster's region.
  - `GCS_PATH:` Replace `<gcs_bucket_path>` with an existing GCS bucket path where diagnostics can be written
      (e.g., `gs://your-ml-diagnostics-bucket/tests`). Ensure the service account has write access to this bucket.
- `google.com/tpu` (Line 71 & 73): Adjust the TPU resource requests/limits to match the chips per node of your machine_type (e.g., 4 for tpu7x-standard-4t).

```yaml
# Line 19 and 29
namespace: ai-workloads # Example

# Line 48
serviceAccountName: workload-identity-k8s-sa # Default

# Line 52 and 53
nodeSelector:
    cloud.google.com/gke-tpu-accelerator: <tpu_accelerator> # ex: tpu-v6e-slice for TPU v6e and tpu7x for TPU 7x
    cloud.google.com/gke-tpu-topology: <tpu_topology>

# Line 57
image: <workload_image>

# Lines 60, 62, 64
env:
- name: PROJECT_ID
  value: <project_id>
- name: REGION
  value: <region>
- name: GCS_PATH
  value: <gcs_bucket_path>

# Lines 71, 73 (adjust based on machine type)
resources:
  requests:
    google.com/tpu: 4 # Example for 4 chips per node
  limits:
    google.com/tpu: 4 # Example
```

### 4. Submit the Job

Apply the modified `sample_job.yaml` to your cluster:

```sh
kubectl apply -f sample_job.yaml
```

### 5. Verify and Monitor

Verify resources created in the user workload namespace:

```sh
kubectl get all -n <user_namespace>
```

Check the logs of the pod. The pod name will start with sample-tpu-jobset-tpu-slice-.

```sh
kubectl logs <pod_name> -n <user_namespace>
```

The logs will show the output of the JAX script. Crucially, they will also contain a URL to the Pantheon: MLrun page for this specific job.

Open the MLrun URL in your browser. Here you can:

- Verify metrics being emitted by the workload.
- After the job completes, find the profiling data under the "Profile" tab.
- Check the GCS bucket path specified in GCS_PATH for written diagnostics files (like XPlane profiles).
