# ML Diagnostics Sample Workload Test

To test a sample ML workload, we need to create a container image and submit a job to utilize that image.

For more information on sample workload tests, refer to the [ML Diagnostics SDK documentation](https://docs.cloud.google.com/tpu/docs/ml-diagnostics/sdk). While that documentation describes execution within a cluster, we will create a Docker image using a `Dockerfile`. This eliminates the need to manually install additional libraries on the cluster nodes, as all required packages are installed as part of the image. The workload will then be deployed and run as a container within the GKE environment.

## Prerequisites

To run a sample job, you need a GKE TPU cluster with ML Diagnostics enabled.

1. Deploy any GKE TPU blueprint with ML Diagnostics enabled. You can find the changes required in the [GKE TPU v6e README](../../examples/gke-tpu-v6e/README.md#understanding-ml-diagnostics-integration).
2. Verify that the cluster was created and configured correctly by following the [Verification section](../../examples/gke-tpu-v6e/README.md#testing-ml-diagnostics-cluster-creation).

Refer to the [ML Diagnostics documentation](https://docs.cloud.google.com/tpu/docs/ml-diagnostics/overview) for more general information.

## Steps to Run Sample Workload

### 1. Create a Docker repository in Artifact Registry

Navigate to the test folder:

```sh
cd tools/ml-diagnostics-test
```

Set environment variables:

```sh
export REGION=<region> # region used in blueprint
export PROJECT_ID=<project_id> # project id used in blueprint
export REPO_NAME=<repo_name> # repo name can be anything
```

Create the repository:

```sh
gcloud artifacts repositories create ${REPO_NAME} \
    --repository-format=docker \
    --project="${PROJECT_ID}" \
    --location="${REGION}"
```

### 2. Build and Push Docker Image

Set the image URI (replace `<image-name>` with your choice):

```sh
export IMAGE_URI="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}/<image-name>:latest"
```

Build and push the image:

```sh
docker build -t "${IMAGE_URI}" .
docker push "${IMAGE_URI}"
```

### 3. Modify `sample_job.yaml`

Update the following placeholders in `sample_job.yaml`:
- Line 19 and 29: replace `<user_namespace>` with your workload namespace from the blueprint.
- Line 44: replace `<k8s-service-account-name>` with your K8s service account name from the blueprint, default is `workload-identity-k8s-sa`.
- Line 48: replace `<tpu_accelerator>` with TPU accelerator type provisioned in your cluster.
- Line 49: replace `<tpu_topology>` with your TPU topology from the blueprint.
- Line 53: replace `#Add Image tag here` with your `IMAGE_URI` generated in step 2.
- Line 56: replace `<project_id>` with your project ID of your cluster.
- Line 58: replace `<region>` with your region of your cluster.
- Line 60: replace `<gcs_bucket_path>` with your existing GCS bucket path (e.g., `gs://my-bucket/path`).

```yaml
# Line 19 and 29
namespace: <user_namespace>

# Line 44
serviceAccountName: <k8s-service-account-name>

# Line 48 and 49
nodeSelector:
    cloud.google.com/gke-tpu-accelerator: <tpu_accelerator> # ex: tpu-v6e-slice for TPU v6e and tpu7x for TPU 7x
    cloud.google.com/gke-tpu-topology: <tpu_topology>

# Line 53
image: #Add Image tag here

# Lines 56, 58, 60
env:
- name: PROJECT_ID
  value: <project_id>
- name: REGION
  value: <region>
- name: GCS_PATH
  value: <gcs_bucket_path>
```

### 4. Submit the Job

Apply the job manifest:

```sh
kubectl apply -f sample_job.yaml
```

### 5. Verify and Monitor

Verify resources created in the user workload namespace:

```sh
kubectl get all -n <user_namespace>
```

Check pod logs in the user workload namespace. The logs will also contain a URL to the Pantheon: MLrun page for this job.

```sh
kubectl logs <pod_name> -n <user_namespace>
```

In the Pantheon: MLrun page, you can verify the metrics being pushed. After the job completes, a profile will be created under the profile tab in the webpage.
