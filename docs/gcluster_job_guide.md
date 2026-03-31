# Gcluster Job Submission Guide

This guide provides a step-by-step process to deploy a GKE cluster, submit a sample Python script as a job using `gcluster job submit` with on-the-fly image building via Crane, and then destroy the cluster.

## 1. Prerequisites (Automated by `gcluster job submit`)

`gcluster job submit` now automates the setup and verification of most prerequisites. The tool will check for required installations and configurations, guide you through interactive steps, and remember successful checks to optimize subsequent runs.

However, a few foundational components are still assumed or require your initial attention:

* **Go (1.20 or later):** Required for building the `gcluster` binary. The `make` command used in step 3 will handle Go module dependencies.
* **Google Cloud SDK (`gcloud`):** While `gcluster job submit` will guide you through authentication and project configuration, the `gcloud` CLI tool itself must be installed and available in your system's PATH. Download and install it from [https://cloud.google.com/sdk/docs/install](https://cloud.google.com/sdk/docs/install).
  * **Manual Authentication:** For interactive steps like `gcloud auth login` or `gcloud auth application-default login`, `gcluster job submit` will detect if you are unauthenticated and provide instructions to run these commands manually in your terminal. This is because these commands typically require browser interaction that cannot be automated.
* **A GCP Project:** You will need a Google Cloud Project with billing enabled and necessary APIs enabled (e.g., Kubernetes Engine API, Artifact Registry API, Cloud Resource Manager API). `gcluster job submit` will prompt you to set a default project if none is configured and will automatically enable necessary APIs like Artifact Registry.
* **Docker:** While `gcluster job submit` uses Crane internally for image building, having Docker installed can be useful for debugging or local container image development. `gcluster job submit` will configure container credential helpers for Google Container Registry and Artifact Registry automatically.
* **`make`:** (Usually pre-installed on Linux/macOS, or install via package manager).

### Automated Prerequisite Checks Overview

When you run `gcluster job submit` for the first time, or if its cached state is stale (after 24 hours) or the `--project` flag changes, the tool will perform the following checks and actions:

* **Google Cloud SDK:** Verifies `gcloud` is installed.
* **GCP Project Configuration:**
  * If `--project` flag is not used, it attempts to infer from your `gcloud` configuration.
  * If no project is configured, it will prompt you to enter your GCP Project ID and automatically configure `gcloud`.
* **Gcloud Authentication:**
  * Checks if `gcloud` is authenticated. If not, it will instruct you to run `gcloud auth login` manually.
  * Checks for Application Default Credentials (ADC). If not configured, it will instruct you to run `gcloud auth application-default login` manually.
* **`kubectl` Installation:**
  * Checks if `kubectl` is installed.
  * If not, it will prompt you to install it via `gcloud components install kubectl`.
  * If `gcloud components install kubectl` fails (e.g., component manager disabled), it will offer to install `kubectl` via `sudo apt-get install kubectl` (for Debian/Ubuntu systems).
* **Container Credential Helper:** Configures Docker to authenticate to Google Container Registry and Artifact Registry.
* **Artifact Registry API:** Ensures `artifactregistry.googleapis.com` is enabled for your project, enabling it automatically if necessary.
* **Kueue Installation:** Checks if Kueue is installed on the cluster. If not, it automatically installs Kueue and configures necessary resources like PriorityClasses, ClusterQueue, and LocalQueue.

**State Persistence:** To avoid redundant checks, `gcluster job submit` saves the successful prerequisite status in `~/.gcluster-job/prereq_state.json`. Checks will only be re-run if this state is older than 24 hours or if you specify a different GCP project.

## 2. Clone the Repository

Clone the `cluster-toolkit` repository to your local machine:

```bash
git clone https://github.com/GoogleCloudPlatform/hpc-toolkit cluster-toolkit
cd cluster-toolkit
```

## 3. Build the `gcluster` Binary

Navigate to the `cluster-toolkit` directory (if not already there) and build the `gcluster` binary:

```bash
make
```

This command compiles the Go source code, including the `gcluster job submit` command, and creates an executable named `gcluster` in the current directory.

## 4. Prepare Sample Application Code

Create a directory named `job_details` and place the following files inside it. This will serve as your build context for the job.

### `cluster-toolkit/job_details/Dockerfile`

```dockerfile
FROM python:3.9-slim

WORKDIR /app

COPY requirements.txt .

RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

CMD ["python", "app.py"]
```

### `cluster-toolkit/job_details/requirements.txt`

(This file can be empty for this simple example, or list any Python dependencies.)

```text
# No specific requirements for this example
```

### `cluster-toolkit/job_details/app.py`

```python
# app.py
print("Hello from the gcluster job submit application!")
print("This is a sample application running on GKE.")
```

## 5. Deploy a GKE Cluster

For this example, we'll deploy a basic GKE cluster using the `hpc-gke.yaml` blueprint.

* **Ensure `gcloud` is configured with your project ID and a region/zone where GKE is available.**

    ```bash
        gcloud config set project <YOUR_GCP_PROJECT_ID>
        gcloud config set compute/region us-central1 # Or your preferred region
        ```

    * **Create the deployment directory:**

    ```bash
    ./gcluster create examples/hpc-gke.yaml --vars="project_id=<YOUR_GCP_PROJECT_ID>,deployment_name=my-test-cluster,region=us-central1,gcp_public_cidrs_access_enabled=false,authorized_cidr=$(curl -s ifconfig.me)/32"
    ```

    *Replace `<YOUR_GCP_PROJECT_ID>` with your actual GCP Project ID.*

* **Deploy the GKE cluster:**

    ```bash
    ./gcluster deploy my-test-cluster
    ```

    *This command will show a Terraform plan. You will be prompted to confirm the changes (type `a` and press Enter).*

    *This deployment process can take a significant amount of time (e.g., 10-20 minutes or more) as it provisions cloud resources.* Wait for the command to complete successfully.

## 6. `gcluster job submit` Command Reference

The `gcluster job submit` command deploys a container image as a job (Kubernetes JobSet) on a GKE cluster, integrated with Kueue. It can use pre-built images or build images on-the-fly using Crane.

### Supported Flags

Here are the flags currently supported by `gcluster job submit`:

* `-i, --image string`: Name of a pre-built container image to run (e.g., `my-project/my-image:tag`). Use this if your image is already pushed to a registry.
* `--base-image string`: Name of the base container image for Crane to build upon (e.g., `python:3.9-slim`). Required when using `--build-context` for an on-the-fly build.
* `-c, --build-context string`: Path to the build context directory for Crane (e.g., `./job_details`). Required with `--base-image`. Crane will automatically look for a `Dockerfile` within this directory.
* `-e, --command string`: Command to execute in the container (e.g., `'python app.py'`). This overrides the `CMD` instruction in your `Dockerfile`. (Required)
* `-a, --accelerator string`: Type of accelerator to request (e.g., `'nvidia-h100-mega-80gb'`). If empty, `gcluster job submit` will auto-discover the optimal accelerator available on the cluster nodes. (Optional)
* `-o, --dry-run-out string`: Path to output the generated Kubernetes manifest instead of applying it directly to the cluster. Useful for inspection.
* `--cluster string`: Name of the GKE cluster to deploy the job to. (Required)
* `--cluster-location string`: Location (Zone or Region) of the GKE cluster. (Required)
* `-p, --project string`: Google Cloud Project ID. If not provided, it will be inferred from your `gcloud` configuration.
* `-f, --platform string`: Target platform for the image build (e.g., `linux/amd64`, `linux/arm64`). Used with `--base-image`. (Default: `linux/amd64`)
* `-w, --name string`: Name of the job (JobSet) to create. This name will be used for Kubernetes resources. (Required)
* `--kueue-queue string`: Name of the Kueue LocalQueue to submit the job to. (Default: Auto-discovered from the cluster)
* `--nodes int`: Number of JobSet replicas (slices). (Default: `1`)
* `--vms-per-slice int`: Number of VMs (pods) per slice. (Default: `1`)
* `--max-restarts int`: Maximum number of restarts for the JobSet before failing. (Default: `1`)
* `--ttl-seconds-after-finished int`: Time (in seconds) to retain the JobSet after it finishes. (Default: `3600` seconds / 1 hour)
* `--mount stringArray`: Mount a storage volume (e.g., `gs://bucket:/data` or `/host/path:/data`). Can be specified multiple times.

## 7. Submit the Sample Job with `gcluster job submit`

Now that the cluster is deployed and your application code is prepared, you can submit your sample Python script as a JobSet job. `gcluster job submit` will automatically build your container image using Crane and push it to Artifact Registry (or Container Registry) in your project.

### Unified Job Submission

Because `gcluster job submit` features auto-discovery, you can use the exact same command to deploy to a standard CPU cluster (like `hpc-gke`) or an accelerated GPU/TPU cluster (like `gke-a3-megagpu`). The orchestrator will automatically query the Kubernetes cluster API to discover the installed Node Accelerators and Kueue Queues, injecting the exact `nvidia.com/gpu` limits your hardware requires.

* **Submit the Job:**

    ```bash
    ./gcluster job submit \
      --project <YOUR_GCP_PROJECT_ID> \
      --cluster my-test-cluster \
      --cluster-location us-central1 \
      --base-image python:3.9-slim \
      --build-context job_details \
      --command "python app.py" \
      --name my-python-app-job
    ```

    *Replace `<YOUR_GCP_PROJECT_ID>` with your actual GCP Project ID.*

    This command will:
    1. Verify/install the JobSet CRD on your cluster.
    2. Auto-discover the Kueue LocalQueue name from the cluster.
    3. Auto-discover the hardware Accelerator Type installed on the cluster nodes and map the necessary resource requests.
    4. Build a container image from `job_details/Dockerfile` using `python:3.9-slim` as the base, and push it to Artifact Registry.
    5. Generate and apply an intelligently configured Kubernetes JobSet manifest to your cluster.

### 7.1 Example: Submit Job with Persistent Storage (Mounting Bucket)

You can mount Cloud Storage buckets or host paths using the `--mount` flag:

```bash
./gcluster job submit \
  --project <YOUR_GCP_PROJECT_ID> \
  --cluster my-test-cluster \
  --cluster-location us-central1 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --command "python app.py" \
  --name my-storage-job \
  --mount "gs://<YOUR_BUCKET_NAME>:/data"
```

## 8. Verify the Job

Verify that the Kubernetes JobSet ran successfully on your GKE cluster.

* **Check JobSet Status:**

    ```bash
    kubectl get jobset --namespace default
    ```

    Look for a JobSet named `my-python-app-job` with a `SUCCEEDED` status in the `CONDITIONS` section.

* **Get Pod Logs:**
    First, get the name of the Pod created by your JobSet:

    ```bash
    kubectl get pods --namespace default -l jobset.sigs.k8s.io/jobset-name=my-python-app-job
    ```

    Note the Pod name (e.g., `my-python-app-job-worker-0-xxxxxx`).

    Then, get the logs from the pod:

    ```bash
    kubectl logs <POD_NAME> --namespace default
    ```

    You should see the output:

    ```text
    Hello from the gcluster job submit application!
    This is a sample application running on GKE.
    ```

## 8. Verify Phase 2 Features (Advanced Scheduling & Lifecycle)

Phase 2 introduces job lifecycle management (`job list`, `job cancel`, `job logs`) and advanced scheduling flags.

### 8.1 List Jobs

You can now list the status of jobs directly through `gcluster`.

```bash
./gcluster job list \
  --project <YOUR_GCP_PROJECT_ID> \
  --cluster my-test-cluster \
  --cluster-location us-central1
```

You should see a table output with `NAME`, `STATUS`, `CREATION_TIME`, and `COMPLETION_TIME`.

### 8.2 Inspect Logs

You can view the logs of your submitted job directly with `gcluster job logs`.

```bash
./gcluster job logs <job-name> \
  --project <YOUR_GCP_PROJECT_ID> \
  --cluster my-test-cluster \
  --cluster-location us-central1
```

This will fetch and print output logs from all containers in the job pods.

### 8.3 Run with Advanced Scheduling Flags

Try running a job with advanced scheduling options.

**Example 1: Target Specific Nodes (Machine Label)**
Use `--machine-label` to target specific hardware (e.g., C2 nodes).

```bash
./gcluster job submit \
  --project <YOUR_GCP_PROJECT_ID> \
  --cluster my-test-cluster \
  --cluster-location us-central1 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --command "python app.py" \
  --name my-machine-job \
  --machine-label "node.kubernetes.io/instance-type=c2-standard-60"
```

**Example 2: Use Placement Policy**
Use `--placement-policy` to specify a GKE Placement Policy (e.g., for compact placement to reduce latency).

```bash
./gcluster job submit \
  ... \
  --name my-compact-job \
  --placement-policy "compact-placement"
```

*(Note: requires a `PlacementPolicy` resource named `compact-placement` to exist on the cluster)*

**Example 3: Pod Failure Policy**
Use `--restart-on-exit-codes` to ignore specific exit codes (e.g., treating exit code 1 as success or retriable non-failure).

```bash
./gcluster job submit \
  ... \
  --name my-robust-job \
  --restart-on-exit-codes 0,1,137
```

(Note: Exit code 0 is always ignored by default)

**Example 4: Private Registry & Service Account**
Use `--image-pull-secret` and `--service-account` for secure jobs.

```bash
./gcluster job submit \
  ... \
  --name my-secure-job \
  --image-pull-secret "my-private-registry-secret" \
  --service-account "my-workload-sa"
```

**Example 5: Explicit Kueue Queue Selection**
Use `--kueue-queue` to submit the job to a specific Kueue LocalQueue.

```bash
./gcluster job submit \
  --project <YOUR_GCP_PROJECT_ID> \
  --cluster my-test-cluster \
  --cluster-location us-central1 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --command "python app.py" \
  --name my-kueue-job \
  --kueue-queue "my-local-queue"
```

(Note: You would need to ensure a Kueue `LocalQueue` named `my-local-queue` is configured on your cluster.)

### 8.4 Cancel Jobs

You can clean up specific job without destroying the entire cluster.

```bash
./gcluster job cancel my-python-app-job \
  --project <YOUR_GCP_PROJECT_ID> \
  --cluster my-test-cluster \
  --cluster-location us-central1
```

Verify it's gone by running `gcluster job list` again.

### 8.5 Job Retention (TTL)

By default, finished jobs are kept for 1 hour (3600 seconds). You can change this using `--ttl-seconds-after-finished`.

```bash
./gcluster job submit ... --ttl-seconds-after-finished 600 # Keep for only 10 minutes
```

### 8.6 Topology & Scheduler

**Example 1: Topology Awareness**
Request a specific TPU slice topology using `--topology`.

```bash
./gcluster job submit \
  --project <YOUR_GCP_PROJECT_ID> \
  --cluster my-test-cluster \
  --cluster-location us-central1 \
  --name my-topology-job \
  --base-image python:3.9-slim \
  --build-context job_details \
  --command "python app.py" \
  --accelerator tpu-v6e-slice \
  --topology 4x4
```

**Example 2: Scheduler Selection**
Use a specific scheduler (e.g., `gke.io/topology-aware-auto`) using `--scheduler`.

```bash
./gcluster job submit \
  ... \
  --name my-scheduler-job \
  --scheduler gke.io/topology-aware-auto
```

## 9. Sophisticated Workloads: MaxText Llama3.1-8B (TPU v6e)

This section describes how to deploy a more complex workload, specifically training a Llama3.1-8B model using MaxText on a TPU v6e cluster.

### 9.1 Prepare MaxText Workload Directory

Create a directory named `maxtext_workload_v6e` and place the following files inside it.

#### `cluster-toolkit/maxtext_workload_v6e/requirements.txt`

```text
# This is your requirements.txt file
```

#### `cluster-toolkit/maxtext_workload_v6e/Dockerfile`

```dockerfile
# Use the recommended base image
FROM us-docker.pkg.dev/cloud-tpu-images/jax-ai-image/tpu:jax0.6.1-rev1

# Set the working directory
WORKDIR /deps

# Install system dependencies
RUN apt-get update && apt-get install -y dnsutils

# Install MaxText dependencies
RUN pip install google-cloud-monitoring

# Clone MaxText
RUN git clone https://github.com/AI-Hypercomputer/maxtext.git /app \
    && cd /app \
    && git checkout tpu-recipes-v0.1.4

# Skip explicit pip install because base image has prerequisites

# Set working directory to MaxText root for the runner
WORKDIR /app

# Copy the wrapper script
COPY run_maxtext.sh /app/run_maxtext.sh
RUN chmod +x /app/run_maxtext.sh

# Entrypoint is left to default or overridden by gcluster
```

#### `cluster-toolkit/maxtext_workload_v6e/run_maxtext.sh`

```bash
#!/bin/bash

# Exit on error
set -e

echo "Starting MaxText Workload..."

# 1. Set environment variables
# Combine all the required XLA flags into LIBTPU_INIT_ARGS
export LIBTPU_INIT_ARGS=" --xla_tpu_scoped_vmem_limit_kib=98304 --xla_tpu_use_minor_sharding_for_major_trivial_input=true --xla_tpu_relayout_group_size_threshold_for_reduce_scatter=1 --xla_tpu_assign_all_reduce_scatter_layout=true --xla_tpu_enable_data_parallel_all_reduce_opt=true --xla_tpu_enable_async_collective_fusion_fuse_all_reduce=false --xla_tpu_enable_sparse_core_collective_offload_all_reduce=true --xla_tpu_enable_all_reduce_offload_tracing=true --xla_tpu_use_tc_device_shape_on_sc=true --xla_sc_enable_instruction_fusion=false --xsc_disjoint_spmem=false --xla_sc_disable_megacore_partitioning=true --2a886c8_chip_config_name=megachip_tccontrol --xla_tpu_enable_all_experimental_scheduler_features=true --xla_tpu_enable_scheduler_memory_pressure_tracking=true --xla_tpu_host_transfer_overlap_limit=24 --xla_tpu_aggressive_opt_barrier_removal=ENABLED --xla_lhs_prioritize_async_depth_over_stall=ENABLED --xla_tpu_enable_ag_backward_pipelining=true --xla_should_allow_loop_variant_parameter_in_chain=ENABLED --xla_should_add_loop_invariant_op_in_chain=ENABLED --xla_max_concurrent_host_send_recv=100 --xla_tpu_scheduler_percent_shared_memory_limit=100 --xla_latency_hiding_scheduler_rerun=2 --xla_jf_spmd_threshold_for_windowed_einsum_mib=1000000"

export JAX_PLATFORMS="tpu,cpu"
export ENABLE_PJRT_COMPATIBILITY=true

# 2. Extract arguments
OUTPUT_DIR=${1}
if [ -z "$OUTPUT_DIR" ]; then
  echo "Error: Output directory argument missing."
  echo "Usage: $0 <output_gcs_bucket>"
  exit 1
fi

MODEL_NAME=${2:-"llama3.1-8b"}

echo "LIBTPU_INIT_ARGS=$LIBTPU_INIT_ARGS"
echo "OUTPUT_DIR=$OUTPUT_DIR"
echo "MODEL_NAME=$MODEL_NAME"

# 3. Run training
python3 -m MaxText.train MaxText/configs/base.yml \
    per_device_batch_size=3 \
    ici_fsdp_parallelism=-1 \
    remat_policy=custom \
    decoder_layer_input=offload \
    out_proj=offload \
    query_proj=offload \
    key_proj=offload \
    value_proj=offload \
    max_target_length=8192 \
    attention=flash \
    use_iota_embed=True \
    dataset_path=gs://max-datasets-rogue \
    dataset_type=synthetic \
    enable_checkpointing=False \
    sa_block_q=2048 \
    sa_block_kv=2048 \
    sa_block_kv_compute=2048 \
    sa_block_q_dkv=2048 \
    sa_block_kv_dkv=2048 \
    sa_block_kv_dkv_compute=2048 \
    sa_block_q_dq=2048 \
    sa_block_kv_dq=2048 \
    sa_use_fused_bwd_kernel=True \
    profiler=xplane \
    skip_first_n_steps_for_profiler=10 \
    profiler_steps=5 \
    steps=20 \
    monitor_goodput=True \
    enable_goodput_recording=True \
    model_name=$MODEL_NAME \
    base_output_directory=$OUTPUT_DIR \
    use_vertex_tensorboard=false
```

#### `cluster-toolkit/maxtext_workload_v6e/build.sh`

```bash
#!/bin/bash

# Get current project
PROJECT=$(gcloud config get-value project)

if [ -z "$PROJECT" ]; then
  echo "Error: Could not determine GCP project. Please run 'gcloud config set project <PROJECT_ID>'"
  exit 1
fi

IMAGE_NAME=gcr.io/$PROJECT/maxtext-runner:latest

echo "Building image $IMAGE_NAME using Cloud Build..."
gcloud builds submit --tag $IMAGE_NAME .

echo "Image built successfully!"
echo "You can now submit the job with:"
echo "  gcluster job submit --image $IMAGE_NAME --command 'bash run_maxtext.sh <OUTPUT_DIR>'"
```

#### `cluster-toolkit/maxtext_workload_v6e/submit.sh`

```bash
#!/bin/bash

# Configuration - UPDATE THESE
CLUSTER_NAME="v6e-xpkgsc"
ZONE="southamerica-west1"
OUTPUT_DIR="gs://gke-aishared-gsc-dev/maxtext_output"

# Look up project
PROJECT=$(gcloud config get-value project)

if [ -z "$PROJECT" ]; then
  echo "Error: Could not determine GCP project. Please run 'gcloud config set project <PROJECT_ID>'"
  exit 1
fi

IMAGE_NAME=gcr.io/$PROJECT/maxtext-runner:latest

echo "Ensuring permissions for $SA_NAME..."
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:v6e-xpkgsc-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/logging.logWriter" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:v6e-xpkgsc-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.admin" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:v6e-xpkgsc-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/monitoring.metricWriter" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:v6e-xpkgsc-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/logging.viewer" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:v6e-xpkgsc-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.objectViewer" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:v6e-xpkgsc-gke-np-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/artifactregistry.reader" --quiet

echo "Submitting MaxText job to cluster $CLUSTER_NAME..."

# Navigate to cluster-toolkit root if running from maxtext_workload_v6e
if [ -f "../gcluster" ]; then
  GCLUSTER="../gcluster"
else
  GCLUSTER="./gcluster"
fi

$GCLUSTER job submit \
    --name maxtext-llama3-1-final-v6e8-2 \
    --cluster $CLUSTER_NAME \
    --cluster-location $ZONE \
    --image $IMAGE_NAME \
    --command "cd /app && pip install psutil jaxtyping tiktoken sentencepiece ray fastapi uvicorn portpicker pydantic ninja Pillow gcsfs omegaconf jsonlines PyYAML safetensors tabulate tensorstore transformers datasets evaluate nltk pandas ml_collections ml_dtypes pathwaysutils orbax grain tensorflow_text tensorflow_datasets tqdm && sed -i 's/use_vertex_tensorboard=false/use_vertex_tensorboard=false run_name=llama3-1-v6e8-test1/g' run_maxtext.sh && bash run_maxtext.sh $OUTPUT_DIR" \
    --accelerator v6e-8 \
    --nodes 1 \
    --vms-per-slice 2 \
    --topology 2x4 \
    --priority medium
    --service-account workload-identity-k8s-sa
```

### 9.2 Build and Submit

```bash
cd maxtext_workload_v6e
./build.sh
./submit.sh
```

### 9.3 Verify Job and Logs

You can verify the job status and check logs using `gcluster` or `kubectl`.

**Using `gcluster`**:

```bash
# List jobs
./gcluster job list --project <YOUR_PROJECT_ID> --cluster v6e-xpkgsc --cluster-location southamerica-west1

# View logs
./gcluster job logs maxtext-llama3-1-final-v6e8-2 --project <YOUR_PROJECT_ID> --cluster v6e-xpkgsc --cluster-location southamerica-west1
```

**Using `kubectl`**:

```bash
# Get pods
kubectl get pods --namespace default -l jobset.sigs.k8s.io/jobset-name=maxtext-llama3-1-final-v6e8-2

# View logs for a specific pod
kubectl logs <POD_NAME> --namespace default
```

## 10. Sophisticated Workloads: MaxText Llama3.1-8B (TPU v7x)

This section describes how to deploy the MaxText workload specifically optimized for TPU v7x (Ironwood) hardware.

### 10.1 Prepare MaxText v7x Workload Directory

Create a directory named `maxtext_workload_v7x` and place the following files inside it.

#### `cluster-toolkit/maxtext_workload_v7x/requirements.txt`

```text
psutil
jaxtyping
tiktoken
sentencepiece
ray
fastapi
uvicorn
portpicker
pydantic
ninja
Pillow
gcsfs
omegaconf
jsonlines
PyYAML
safetensors
tabulate
tensorstore
transformers
datasets
evaluate
nltk
pandas
ml_collections
ml_dtypes
pathwaysutils
orbax
grain
tensorflow_text
tensorflow_datasets
tqdm
```

#### `cluster-toolkit/maxtext_workload_v7x/Dockerfile`

```dockerfile
# Use the recommended base image for TPU7x
FROM us-docker.pkg.dev/cloud-tpu-images/jax-ai-image/tpu:jax0.8.2-rev1

# Set the working directory
WORKDIR /deps

# Install system dependencies
RUN apt-get update && apt-get install -y dnsutils

# Install MaxText dependencies
RUN pip install google-cloud-monitoring

# Install Python requirements
COPY requirements.txt .
RUN pip install -r requirements.txt

# Clone MaxText
RUN git clone https://github.com/AI-Hypercomputer/maxtext.git /app \
    && cd /app \
    && git checkout maxtext-tutorial-v1.0.0

# Set working directory to MaxText root for the runner
WORKDIR /app

# Copy the wrapper script
COPY run_maxtext.sh /app/run_maxtext.sh
RUN chmod +x /app/run_maxtext.sh

# Entrypoint is left to default or overridden by gcluster
```

#### `cluster-toolkit/maxtext_workload_v7x/run_maxtext.sh`

```bash
#!/bin/bash

# Exit on error
set -e

echo "Starting MaxText Workload..."

# 1. Set environment variables
export ENABLE_PATHWAYS_PERSISTENCE='1'
# Combine all the required XLA flags into LIBTPU_INIT_ARGS
export LIBTPU_INIT_ARGS=" --xla_tpu_scoped_vmem_limit_kib=61440 --xla_tpu_bf16_emission_mode=NATIVE_EMISSION --xla_tpu_enable_sparse_core_collective_offload_all_reduce=true --xla_tpu_use_single_sparse_core_for_all_gather_offload=true "

export JAX_PLATFORMS="tpu,cpu"
export ENABLE_PJRT_COMPATIBILITY=true
export PYTHONPATH=$PYTHONPATH:$(pwd)/src
export JAX_TRACEBACK_FILTERING=off

# 2. Extract arguments
OUTPUT_DIR=${1}
if [ -z "$OUTPUT_DIR" ]; then
  echo "Error: Output directory argument missing."
  echo "Usage: $0 <output_gcs_bucket>"
  exit 1
fi

MODEL_NAME=${2:-"llama3.1-8b"}

echo "LIBTPU_INIT_ARGS=$LIBTPU_INIT_ARGS"
echo "OUTPUT_DIR=$OUTPUT_DIR"
echo "MODEL_NAME=$MODEL_NAME"

# 3. Run training
python3 src/MaxText/train.py src/MaxText/configs/base.yml \
    model_name=$MODEL_NAME \
    skip_jax_distributed_system=False \
    dtype=bfloat16 \
    per_device_batch_size=1 \
    ici_fsdp_parallelism=64 \
    max_target_length=4096 \
    profiler=xplane \
    profile_periodically_period=10000 \
    async_checkpointing=False \
    enable_checkpointing=False \
    use_iota_embed=True \
    remat_policy=custom \
    decoder_layer_input=offload \
    query_proj=offload \
    key_proj=offload \
    value_proj=offload \
    out_proj=offload \
    dataset_type=synthetic \
    opt_type=adamw \
    mu_dtype=bfloat16 \
    tokenizer_type=tiktoken \
    tokenizer_path=assets/tokenizer_llama3.tiktoken \
    sa_use_fused_bwd_kernel=True \
    attention=flash \
    steps=30 \
    base_output_directory=$OUTPUT_DIR \
    use_vertex_tensorboard=false
```

#### `cluster-toolkit/maxtext_workload_v7x/build.sh`

```bash
#!/bin/bash

# Get current project
PROJECT=$(gcloud config get-value project)

if [ -z "$PROJECT" ]; then
  echo "Error: Could not determine GCP project. Please run 'gcloud config set project <PROJECT_ID>'"
  exit 1
fi

IMAGE_NAME=gcr.io/$PROJECT/maxtext-runner:latest

echo "Building image $IMAGE_NAME using Cloud Build..."
gcloud builds submit --tag $IMAGE_NAME .

echo "Image built successfully!"
echo "You can now submit the job with:"
echo "  gcluster job submit --image $IMAGE_NAME --command 'bash run_maxtext.sh <OUTPUT_DIR>'"
```

#### `cluster-toolkit/maxtext_workload_v7x/submit.sh`

```bash
#!/bin/bash

# Configuration - UPDATE THESE
CLUSTER_NAME="tpu7xpkv"
ZONE="us-central1-c"
OUTPUT_DIR="gs://gke-aishared-gsc-dev/maxtext_output_7x"

# Look up project
PROJECT=$(gcloud config get-value project)

if [ -z "$PROJECT" ]; then
  echo "Error: Could not determine GCP project. Please run 'gcloud config set project <PROJECT_ID>'"
  exit 1
fi

IMAGE_NAME=gcr.io/$PROJECT/maxtext-runner:latest

echo "Ensuring permissions for tpu7xpkv-gke-wl-sa..."
# Note: Ensure the SA matches the one created by the 7x blueprint (tpu7xpkv-gke-wl-sa)
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7xpkv-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/logging.logWriter" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7xpkv-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.admin" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7xpkv-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/monitoring.metricWriter" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7xpkv-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/logging.viewer" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7xpkv-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.objectViewer" --quiet

echo "Ensuring permissions for node pool service account..."
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7xpkv-gke-np-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/artifactregistry.reader" --quiet
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7xpkv-gke-np-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.objectViewer" --quiet

echo "Submitting MaxText job to cluster $CLUSTER_NAME..."

# Navigate to cluster-toolkit root if running from maxtext_workload_v7x
if [ -f "../gcluster" ]; then
  GCLUSTER="../gcluster"
else
  GCLUSTER="./gcluster"
fi

$GCLUSTER job submit \
    --name maxtext-llama3-1-final-tpu7x-32 \
    --cluster $CLUSTER_NAME \
    --cluster-location us-central1 \
    --image $IMAGE_NAME \
    --command "cd /app && sed -i 's/use_vertex_tensorboard=false/use_vertex_tensorboard=false run_name=llama3-1-7x-test1/g' run_maxtext.sh && bash run_maxtext.sh $OUTPUT_DIR" \
    --accelerator tpu7x-32 \
    --nodes 1 \
    --vms-per-slice 8 \
    --topology 2x4x4 \
    --priority medium \
    --service-account workload-identity-k8s-sa
```

### 10.2 Build and Submit

```bash
cd maxtext_workload_v7x
./build.sh
./submit.sh
```

### 10.3 Verify Job and Logs

TPU v7x utilizes Megacore, which initializes 2 logical devices per chip. For a 32-chip slice (2x4x4 topology), you should see 64 logical devices in the logs.

**Using `gcluster`**:

```bash
# List jobs
./gcluster job list --project <YOUR_PROJECT_ID> --cluster tpu7xpkv --cluster-location us-central1

# View logs
./gcluster job logs maxtext-llama3-1-final-tpu7x-32 --project <YOUR_PROJECT_ID> --cluster tpu7xpkv --cluster-location us-central1
```

**Verification Highlights**:
In the logs, look for successful JAX initialization and step processing:

```text
System Information: Jax Version: 0.8.2
System Information: Jax Backend: PJRT C API
TFRT TPU7x
Num_devices: 64, shape (1, 1, 64, 1, 1, 1, 1, 1, 1, 1, 1, 1)
...
completed step: 4, seconds: 1.182, TFLOP/s/device: 167.154, Tokens/s/device: 3464.384, loss: 10.187
completed step: 5, seconds: 1.186, TFLOP/s/device: 166.597, Tokens/s/device: 3452.840, loss: 9.184
```

## 11. Cleanup

To avoid incurring unnecessary costs, destroy the deployed GKE cluster and its resources:

```bash
./gcluster destroy my-test-cluster
```

*You will be prompted to confirm the destruction (type `a` and press Enter).*
