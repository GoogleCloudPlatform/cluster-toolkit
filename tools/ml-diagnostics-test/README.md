# ML Diagnostics Sample Workload Test

To test a sample ML workload, we need to create a container image and submit a job to utilize that image.

For more information on sample workload tests, refer to the [ML Diagnostics SDK documentation](https://docs.cloud.google.com/tpu/docs/ml-diagnostics/sdk). While that documentation describes execution within a cluster, we will create a Docker image using a `Dockerfile`. This eliminates the need to manually install additional libraries on the cluster nodes, as all required packages are installed as part of the image. The workload will then be deployed and run as a container within the GKE environment.

## Prerequisites

To run a sample job, you need a GKE TPU cluster with ML Diagnostics enabled.

1. Deploy any GKE TPU blueprint with ML Diagnostics enabled. You can find the changes required in the [GKE TPU v6e README](../../examples/gke-tpu-v6e/README.md#understanding-ml-diagnostics-integration).
2. Verify that the cluster was created and configured correctly by following the [Verification section](../../examples/gke-tpu-v6e/README.md#testing-ml-diagnostics-cluster-creation).

Refer to the [ML Diagnostics documentation](https://docs.cloud.google.com/tpu/docs/ml-diagnostics/overview) for more general information.

## Steps to Run Sample Workload

### 1. Modify `sample_workload/sample_workload.py`

In line ~73, in the `machinelearning_run(..)` section, replace:
- `<project-name>` with project name of your cluster.
- `<region>` with region of your cluster.
- `<existing-gcs-bucket-path>` with your existing GCS bucket path (e.g., `gs://my-bucket/path`).

### 2. Create a Docker repository in Artifact Registry

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

### 3. Build and Push Docker Image

Set the image URI (replace `<image-name>` with your choice):

```sh
export IMAGE_URI="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}/<image-name>:latest"
```

Build and push the image:

```sh
docker build -t "${IMAGE_URI}" .
docker push "${IMAGE_URI}"
```

### 4. Modify `sample_job.yaml`

Update the following placeholders in `sample_job.yaml`:
- Line 19 and 29: replace `<workload_namespace>` with your workload namespace from the blueprint.
- Line 44: replace `<k8s-service-account-name>` with your K8s service account name from the blueprint, default is `workload-identity-k8s-sa`.
- Line 48: replace `<tpu_accelerator>` with TPU accelerator type provisioned in your cluster.
- Line 49: replace `<tpu_topology>` with your TPU topology from the blueprint.
- Line 53: replace `#Add Image tag here` with your `IMAGE_URI` generated in step 3.

```yaml
# Line 19 and 29
namespace: <workload_namespace>

# Line 44
serviceAccountName: <k8s-service-account-name>

# Line 48 and 49
nodeSelector:
    cloud.google.com/gke-tpu-accelerator: <tpu_accelerator> # ex: tpu-v6e-slice for TPU v6e and tpu7x for TPU 7x
    cloud.google.com/gke-tpu-topology: <tpu_topology>

# Line 53
image: #Add Image tag here
```

### 5. Submit the Job

Apply the job manifest:

```sh
kubectl apply -f sample_job.yaml
```

### 6. Verify and Monitor

Verify resources created in the workload namespace:

```sh
kubectl get all -n <workload_namespace>
```

Check pod logs in the workload namespace. The logs will also contain a URL to the Pantheon: MLrun page for this job.

```sh
kubectl logs <pod_name> -n <workload_namespace>
```

In the Pantheon: MLrun page, you can verify the metrics being pushed. After the job completes, a profile will be created under the profile tab in the webpage.
