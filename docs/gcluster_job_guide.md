# Gcluster Job Submission Guide

This guide provides a step-by-step process to deploy a GKE cluster, submit a sample Python script as a job using `gcluster job submit` with on-the-fly image building, and then destroy the cluster.

## 1. Prerequisites

Before you begin, ensure you have installed `gcluster` and set up your environment by following the instructions in the [cluster-toolkit README.md](../README.md#using-the-pre-built-bundle-recommended).

### Conditional Prerequisites (For On-the-Fly Builds)

If you use `--build-context` to build images on-the-fly, you must set:

* `GCLUSTER_IMAGE_REPO`: The name of your Artifact Registry repository only (e.g., `gcluster-repo`). The tool will automatically construct the full path using the cluster's region and project ID.
* `USER` or `USERNAME`: Used for unique image tagging (usually set automatically by your OS).

> [!NOTE]
> ### Automated Prerequisite Checks Overview
>
> When you run `gcluster job submit` (or other job commands), the tool will check for:
>
> * **Google Cloud SDK**: Verifies `gcloud` is installed.
> * **Gcloud Authentication**: Checks if authenticated and if Application Default Credentials (ADC) are valid.
> * **`kubectl` Installation**: Checks if `kubectl` is installed.
> * **GKE Auth Plugin**: Checks if `gke-gcloud-auth-plugin` is installed.
> * **Container Credential Helper**: Checks if Docker is configured for GCR and Artifact Registry.
> * **Artifact Registry API**: Checks if `artifactregistry.googleapis.com` is enabled.
>
> If any required dependencies are missing or unconfigured, `gcluster` will identify them and print the necessary installation or remediation commands directly to your console for review and execution
>
> Successful checks are remembered in `~/.gcluster/job_prereq_state.json` to optimize subsequent runs. Checks are re-run if the state is older than 24 hours or if you switch projects.

## 2. Prepare Sample Application Code

Create a directory named `job_details` and place your application files inside it. This will serve as your build context for the job. The tool will package all files in this directory and add them to the image.

### `cluster-toolkit/job_details/app.py`

```python
# app.py
print("Hello from the gcluster job submit application!")
print("This is a sample application running on GKE.")
```

> [!NOTE]
> This on-the-fly image build does not execute a Dockerfile. It simply copies the contents of the build context directory into the image. If you need to install dependencies, make sure they are already present in your `--base-image`.

## 3. Deploy a GKE Cluster

For this example, we'll deploy a basic GKE cluster using the `hpc-gke.yaml` blueprint.

> [!TIP]
> **Configuring gcloud Defaults**
>
> While not strictly required for this tutorial (as we pass variables explicitly), you can configure your default project and region in `gcloud` for convenience:
>
> ```bash
> gcloud config set project <PROJECT_ID>
> gcloud config set compute/region <REGION/ZONE> # Or your preferred region
> ```

### 3.1 Create the Deployment Directory

Create the deployment directory using the `hpc-gke.yaml` blueprint:

```bash
./gcluster create examples/hpc-gke.yaml --vars="project_id=<PROJECT_ID>,deployment_name=<CLUSTER_NAME>,region=<REGION/ZONE>,gcp_public_cidrs_access_enabled=false,authorized_cidr=$(curl -s ifconfig.me)/32"
```

*Note: Please ensure that you replace all placeholders enclosed in angle brackets (such as `<PROJECT_ID>`, `<CLUSTER_NAME>`, and `<REGION/ZONE>`) with your actual environment values before executing the command.*

### 3.2 Deploy the GKE Cluster

Deploy the GKE cluster:

```bash
./gcluster deploy <CLUSTER_NAME>
```

*This command will show a Terraform plan. You will be prompted to confirm the changes (type `a` and press Enter).*

*This deployment process can take a significant amount of time (e.g., 10-20 minutes or more) as it provisions cloud resources.* Wait for the command to complete successfully.

## 4. Submit the Sample Job

Now that the cluster is deployed and your application code is prepared, you can submit your sample Python script as a JobSet job. `gcluster job submit` will automatically build your container image and push it to Artifact Registry in your project.

> [!IMPORTANT]
> The image will be pushed to a regional Artifact Registry endpoint: `<region>-docker.pkg.dev/<project>/<GCLUSTER_IMAGE_REPO>/<user>-runner:<tag>`.

* You **must** set the `GCLUSTER_IMAGE_REPO` environment variable to specify the name of the Artifact Registry repository when using `--build-context` for on-the-fly builds (e.g., `export GCLUSTER_IMAGE_REPO=gcluster-repo`). The tool will automatically construct the full path using the cluster's region and project ID. The command will fail fast if this variable is not set. The repository **must exist** before submitting the job. If it does not exist, you can create it with:

    ```bash
    gcloud artifacts repositories create <REPOSITORY_NAME> \
        --repository-format=docker \
        --location=<REGION>
    ```

* You **must** have either `USER` or `USERNAME` environment variable set when using `--build-context` (usually set automatically by your OS). `gcluster` uses this to ensure unique image tagging (e.g., `my-user-runner:tag`). The command will fail if both are missing.

### 4.1 Unified Job Submission

By specifying the `--compute-type` flag, you can use the exact same command to target a standard CPU cluster (using a full GCE machine type like `n2-standard-32`), an accelerated GPU cluster (using a GKE accelerator type like `nvidia-l4`), or a TPU cluster (using a shorthand string representing total chips/cores like `v6e-8`). The tool will automatically resolve the machine type, calculate `num-nodes`, and deduce the correct TPU topology if needed.

> [!TIP]
> **Simplify Commands with Configuration**: You can set these values once using the configuration command and omit them from subsequent commands:
>
> ```bash
> ./gcluster job config set project <PROJECT_ID>
> ./gcluster job config set cluster <CLUSTER_NAME>
> ./gcluster job config set location <REGION/ZONE>
> ```
>
> To view your current configuration, run:
>
> ```bash
> ./gcluster job config list
> ```

### 4.2 Submit the Job

```bash
./gcluster job submit \
  --project <PROJECT_ID> \
  --cluster <CLUSTER_NAME> \
  --location <REGION/ZONE> \
  --base-image python:3.9-slim \
  --build-context job_details \
  --command "python app.py" \
  --name my-python-app-job \
  --compute-type n2-standard-32
```

*Note: Please ensure that you replace all placeholders enclosed in angle brackets (such as `<PROJECT_ID>`, `<CLUSTER_NAME>`, and `<REGION/ZONE>`) with your actual environment values before executing the command.*

This command will:
1. Verify/install the JobSet CRD on your cluster.
2. Auto-discover the Kueue LocalQueue name from the cluster.
3. Use the compute type installed on the cluster nodes and map the necessary resource requests.
4. Build a container image from the job_details directory using python:3.9-slim as the base, and push it to Artifact Registry.
5. Generate and apply an intelligently configured Kubernetes JobSet manifest to your cluster.

*Note: The following examples assume you have configured your default project, cluster, and location using `./gcluster job config set`.*

### 4.3 Example for Multi-Slice GPU Job

If you want to run a job across multiple groups of GPU nodes (e.g., 2 groups of 4 nodes each), you can use `--num-slices` and `--num-nodes`:

```bash
./gcluster job submit \
  --image us-docker.pkg.dev/my-project/my-repo/my-image:latest \
  --command "python train.py" \
  --name my-gpu-job \
  --compute-type l4-1 \
  --num-slices 2 \
  --num-nodes 4
```

*This creates a JobSet with 2 replicas, each having 4 pods, totaling 8 nodes.*

### 4.4 Example: Submit Job with Persistent Storage (Mounting Bucket)

You can mount Cloud Storage buckets or host paths using the `--mount` flag. By default, mounts are read-only. You can specify read-write mode by appending `:rw` to the mount string:

```bash
./gcluster job submit \
  --name my-storage-job \
  --command "python app.py" \
  --compute-type n2-standard-32 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --mount "gs://<YOUR_BUCKET_NAME>:/data:rw"
```

## 5. Verify the Job

Verify that the Kubernetes JobSet ran successfully on your GKE cluster.

* **Check Job Status:**
    You can check the status of your submitted job directly with `gcluster job list`:

    ```bash
    ./gcluster job list
    ```

    Look for `my-python-app-job` with a `Succeeded` status.

* **Get Job Logs:**
    You can view the logs of your submitted job directly with `gcluster job logs`:

    ```bash
    ./gcluster job logs my-python-app-job
    ```

    You should see the output:

    ```text
    Hello from the gcluster job submit application!
    This is a sample application running on GKE.
    ```

* **Cancel Jobs:**
    You can clean up a specific job without destroying the entire cluster:

    ```bash
    ./gcluster job cancel my-python-app-job
    ```

    Verify it's gone by running `gcluster job list` again.

## 6. Advanced Workloads

*Note: The following examples assume you have configured your default project, cluster, and location using `./gcluster job config set`.*

### 6.1 Run with Advanced Scheduling Flags

Try running a job with advanced scheduling options.

**Example 1: Target a Specific Node Pool (Node Constraint)**
Use `--node-constraint` to target a specific node pool. This maps to node labels in GKE and aligns with SLURM's `--constraint` flag for future compatibility.

```bash
./gcluster job submit \
  --name my-nodepool-job \
  --command "python app.py" \
  --compute-type c2-standard-60 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --node-constraint "cloud.google.com/gke-nodepool=my-custom-nodepool"
```

**Example 2: Use Placement Policy**
Use `--placement-policy` to specify a GCE Placement Policy (e.g., for compact placement to reduce latency).

```bash
./gcluster job submit \
  ... \
  --name my-compact-job \
  --placement-policy "compact-placement"
```

*(Note: requires a `PlacementPolicy` resource named `compact-placement` to exist on the cluster)*

**Example 3: Pod Failure Policy**
Use `--restart-on-exit-codes` to specify retriable exit codes at the pod level (these do not count against the `restarts` budget).

```bash
./gcluster job submit \
  ... \
  --name my-robust-job \
  --restart-on-exit-codes 1,137
```

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
Use `--queue` to submit the job to a specific Kueue LocalQueue.

```bash
./gcluster job submit \
  --name my-kueue-job \
  --command "python app.py" \
  --compute-type n2-standard-32 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --queue "my-local-queue"
```

(Note: You would need to ensure a Kueue `LocalQueue` named `my-local-queue` is configured on your cluster.)

### 6.3 Job Retention (TTL)

By default, finished jobs are kept for 1 hour. You can change this using `--gke-ttl-after-finished` and pass flexible durations.

```bash
./gcluster job submit ... --gke-ttl-after-finished 10m # Keep for only 10 minutes
./gcluster job submit ... --gke-ttl-after-finished 2h  # Keep for 2 hours
```

### 6.4 Graceful Termination (Grace Period)

You can give your workloads a buffer period to save checkpoints or perform cleanups before they are forcefully killed using `--grace-period`.

```bash
./gcluster job submit ... --grace-period 2m # Allow 2 minutes for cleanup
```

### 6.5 Topology & Scheduler

**Example 1: Topology Awareness**
Request a specific TPU slice topology using `--topology`.

```bash
./gcluster job submit \
  --project <PROJECT_ID> \
  --cluster <CLUSTER_NAME> \
  --location <REGION/ZONE> \
  --name my-topology-job \
  --base-image python:3.9-slim \
  --build-context job_details \
  --command "python app.py" \
  --compute-type tpu-v6e-slice \
  --topology 4x4
```

**Example 2: Scheduler Selection**
Use a specific GKE scheduler (e.g., `gke.io/topology-aware-auto`) using `--gke-scheduler`.

```bash
./gcluster job submit \
  --project <PROJECT_ID> \
  --cluster <CLUSTER_NAME> \
  --location <REGION/ZONE> \
  --name my-scheduler-job \
  --command "python app.py" \
  --compute-type n2-standard-32 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --gke-scheduler gke.io/topology-aware-auto
```

## 7. Sophisticated Workloads: MaxText

### 7.1 Llama3.1-8B on TPU v6e

This section describes how to deploy a more complex workload, specifically training a Llama3.1-8B model using MaxText on a TPU v6e cluster.

#### 7.1.1 Prepare MaxText Workload Directory

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
CLUSTER_NAME="v6e-cluster"
LOCATION="us-central1"
OUTPUT_DIR="gs://$PROJECT/maxtext_output"
SA_NAME="v6e-cluster-gke-wl-sa"

# Look up project
PROJECT=$(gcloud config get-value project)

if [ -z "$PROJECT" ]; then
  echo "Error: Could not determine GCP project. Please run 'gcloud config set project <PROJECT_ID>'"
  exit 1
fi

IMAGE_NAME=gcr.io/$PROJECT/maxtext-runner:latest

echo "Ensuring permissions for $SA_NAME..."
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:$SA_NAME@${PROJECT}.iam.gserviceaccount.com" --role="roles/logging.logWriter" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:$SA_NAME@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.admin" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:$SA_NAME@${PROJECT}.iam.gserviceaccount.com" --role="roles/monitoring.metricWriter" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:$SA_NAME@${PROJECT}.iam.gserviceaccount.com" --role="roles/logging.viewer" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:$SA_NAME@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.objectViewer" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:$SA_NAME@${PROJECT}.iam.gserviceaccount.com" --role="roles/artifactregistry.reader" --quiet > /dev/null

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
    --location $LOCATION \
    --image $IMAGE_NAME \
    --command "cd /app && pip install psutil jaxtyping tiktoken sentencepiece ray fastapi uvicorn portpicker pydantic ninja Pillow gcsfs omegaconf jsonlines PyYAML safetensors tabulate tensorstore transformers datasets evaluate nltk pandas ml_collections ml_dtypes pathwaysutils orbax grain tensorflow_text tensorflow_datasets tqdm && sed -i 's/use_vertex_tensorboard=false/use_vertex_tensorboard=false run_name=llama3-1-v6e8-test1/g' run_maxtext.sh && bash run_maxtext.sh $OUTPUT_DIR" \
    --compute-type v6e-8 \
    --num-slices 1 \
    --topology 2x4 \
    --priority medium \
    --service-account workload-identity-k8s-sa
```

#### 7.1.2 Build and Submit

```bash
cd maxtext_workload_v6e
./build.sh
./submit.sh
```

#### 7.1.3 Verify Job and Logs

You can verify the job status and check logs using `gcluster` or `kubectl`.

**Using `gcluster`**:

```bash
# List jobs
./gcluster job list --project $PROJECT --cluster $CLUSTER_NAME --location $LOCATION

# View logs
./gcluster job logs maxtext-llama3-1-final-v6e8-2 --project $PROJECT --cluster $CLUSTER_NAME --location $LOCATION
```

**Using `kubectl`**:

```bash
# Get pods
kubectl get pods --namespace default -l jobset.sigs.k8s.io/jobset-name=maxtext-llama3-1-final-v6e8-2

# View logs for a specific pod
kubectl logs <POD_NAME> --namespace default
```

### 7.2 Llama3.1-8B on TPU v7x

This section describes how to deploy the MaxText workload specifically optimized for TPU v7x (Ironwood) hardware.

#### 7.2.1 Prepare MaxText v7x Workload Directory

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
CLUSTER_NAME="tpu7x-cluster"
LOCATION="<REGION/ZONE>"
OUTPUT_DIR="gs://$PROJECT/maxtext_output_7x"

# Look up project
PROJECT=$(gcloud config get-value project)

if [ -z "$PROJECT" ]; then
  echo "Error: Could not determine GCP project. Please run 'gcloud config set project <PROJECT_ID>'"
  exit 1
fi

IMAGE_NAME=gcr.io/$PROJECT/maxtext-runner:latest

echo "Ensuring permissions for tpu7x-cluster-gke-wl-sa..."
# Note: Ensure the SA matches the one created by the 7x blueprint (tpu7x-cluster-gke-wl-sa)
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7x-cluster-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/logging.logWriter" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7x-cluster-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.admin" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7x-cluster-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/monitoring.metricWriter" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7x-cluster-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/logging.viewer" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7x-cluster-gke-wl-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.objectViewer" --quiet > /dev/null

echo "Ensuring permissions for node pool service account..."
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7x-cluster-gke-np-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/artifactregistry.reader" --quiet > /dev/null
gcloud projects add-iam-policy-binding $PROJECT --member="serviceAccount:tpu7x-cluster-gke-np-sa@${PROJECT}.iam.gserviceaccount.com" --role="roles/storage.objectViewer" --quiet > /dev/null

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
    --location $LOCATION \
    --image $IMAGE_NAME \
    --command "cd /app && sed -i 's/use_vertex_tensorboard=false/use_vertex_tensorboard=false run_name=llama3-1-7x-test1/g' run_maxtext.sh && bash run_maxtext.sh $OUTPUT_DIR" \
    --compute-type tpu7x-32 \
    --num-slices 1 \
    --topology 2x4x4 \
    --priority medium \
    --service-account workload-identity-k8s-sa
```

To disable parallel containers and use a single container per VM, add the `--gke-disable-parallel-containers` flag:

```bash
$GCLUSTER job submit \
    --name maxtext-llama3-1-final-tpu7x-32 \
    --cluster $CLUSTER_NAME \
    --location $LOCATION \
    --image $IMAGE_NAME \
    --command "cd /app && sed -i 's/use_vertex_tensorboard=false/use_vertex_tensorboard=false run_name=llama3-1-7x-test1/g' run_maxtext.sh && bash run_maxtext.sh $OUTPUT_DIR" \
    --compute-type tpu7x-32 \
    --num-slices 1 \
    --topology 2x4x4 \
    --priority medium \
    --service-account workload-identity-k8s-sa \
    --gke-disable-parallel-containers
```

#### 7.2.2 Build and Submit

```bash
cd maxtext_workload_v7x
./build.sh
./submit.sh
```

#### 7.2.3 Verify Job and Logs

TPU v7x utilizes Megacore, which initializes 2 logical devices per chip. For a 32-chip slice (2x4x4 topology), you should see 64 logical devices in the logs.

**Using `gcluster`**:

```bash
# List jobs
./gcluster job list --project $PROJECT --cluster $CLUSTER_NAME --location $LOCATION

# View logs
./gcluster job logs maxtext-llama3-1-final-tpu7x-32 --project $PROJECT --cluster $CLUSTER_NAME --location $LOCATION
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

## 8. `gcluster job` Command Reference

### 8.1 Common Flags
*These flags are common to almost all `gcluster job` subcommands (except `config`). They can be set as defaults via `config set`.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `-c, --cluster` | `string` | Name of the target GKE cluster. |
| `-l, --location` | `string` | Google Cloud location (Zone or Region) of the GKE cluster. |
| `-p, --project` | `string` | Google Cloud Project ID. |

### 8.2 Configuration Commands
*Use these commands to manage persistent defaults for your job submissions, avoiding the need to pass common flags repeatedly.*

#### `gcluster job config set [key] [value]`
Sets a persistent configuration property.

* **Supported Keys:**
  * `project`: Google Cloud Project ID
  * `cluster`: GKE Cluster Name
  * `location`: GKE Cluster Location (region or zone)

**Example:**

```bash
./gcluster job config set project my-awesome-project
```

#### `gcluster job config list`
Lists all persistent configuration properties currently set.

### 8.3 `submit` Flags
The `gcluster job submit` command deploys a container image as a job (Kubernetes JobSet) on a GKE cluster, integrated with Kueue for advanced queuing. It can use pre-built images or build images on-the-fly without a local Docker daemon (powered internally by the [Crane](https://github.com/google/go-containerregistry/blob/main/cmd/crane/README.md) container utility).

#### 8.3.1 General Flags
*These flags control core workload execution, identity, and basic build context options regardless of the underlying hardware paradigm.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `-n, --name` | `string` | Name of the job (JobSet) to create. Used for Kubernetes resources. *(Required)* |
| `-e, --command` | `string` | Command to execute inside the container (e.g., `'python app.py'`). *(Required)* |
| `--compute-type` | `string` | The hardware target for the job. Accepts a full GCE machine type (e.g., 'n2-standard-32'), a GKE accelerator type (e.g., 'nvidia-l4'), or a TPU shorthand string representing total chips/cores (e.g., 'v6e-8'). *(Required)* The tool will automatically resolve the machine type, calculate num-nodes, and deduce the correct TPU topology if needed. |
| `-i, --image` | `string` | Full registry path of a pre-built container image to run. |
| `-B, --base-image` | `string` | Name of the base container image to build upon (e.g., `python:3.9-slim`). |
| `-b, --build-context` | `string` | Path to the local build context directory for on-the-fly image builds. |
| `-f, --platform` | `string` | Target platform architecture for the image build (Default: `linux/amd64`). |
| `-o, --dry-run-out` | `string` | Local path to save the generated Kubernetes manifest instead of applying it. |
| `--num-slices` | `int` | Number of independent groups/slices to use (Default: `1`). |
| `--num-nodes` | `int` | Number of nodes to use per group/slice (Default: `1`). Auto-calculated for TPUs based on topology. |
| `--restarts` | `int` | Maximum number of restarts allowed for the JobSet before marked as failed (Default: `1`). |
| `--mount` | `stringArray` | Mount storage volumes or buckets using the `<src>:<dest>[:<mode>]` format. |
| `--await-job-completion` | `bool` | If true, the CLI waits for the job to complete before exiting. |
| `--timeout` | `string` | Time to wait for job completion (e.g., `1h`, `10m`). Used with `--await-job-completion`. |
| `--verbose` | `bool` | Enable verbose logging for the workload. |

*(Note: `--cluster`, `--location`, and `--project` are also supported as common flags, see 8.1)*

#### 8.3.2 TPU Related Flags
*Use these flags to orchestrate specialized TPU multi-slice topologies or leverage the Pathways distributed execution framework.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `--topology` | `string` | TPU slice topology (e.g., `2x2x1`, `2x4`, `2x4x4`). Required if `--num-nodes` is omitted. |
| `--pathways` | `flag` | If present, generates a manifest tailored for a Pathways distributed job paradigm. |
| `--pathways-gcs-location` | `string` | GCS bucket location to store Pathways artifacts. *(Required when --pathways is set)* |
| `--pathways-proxy-server-image` | `string` | Container image for the Pathways proxy server. |
| `--pathways-server-image` | `string` | Container image for the Pathways resource manager / server. |
| `--pathways-worker-image` | `string` | Container image for the Pathways workers. |
| `--pathways-headless` | `flag` | If present, the user workload container will not be deployed in the `pathways-head` job. |
| `--pathways-elastic-slices` | `int` | Configures the number of elastic slices for resilient training scaling. |
| `--pathways-max-slice-restarts` | `int` | Maximum number of times the workers within a single slice can be restarted. |
| `--pathways-proxy-args` | `string` | Arbitrary additional command-line arguments passed to `pathways-proxy`. |
| `--pathways-server-args` | `string` | Arbitrary additional command-line arguments passed to `pathways-rm`. |
| `--pathways-worker-args` | `string` | Arbitrary additional command-line arguments passed to `pathways-worker`. |
| `--pathways-colocated-python-sidecar-image` | `string` | Image for an optional Python-based sidecar container running alongside workers. |
| `--pathways-head-np` | `string` | The node pool name to target for the Pathways head job. |

#### 8.3.3 GPU Related Flags
*Use these flags to tune specialized multi-GPU topologies and related node parameters.*

> [!NOTE]
> Currently, GPU workloads leverage the standard **General Flags** (such as passing `--compute-type nvidia-l4` or `--compute-type nvidia-h100-8px`) and **GKE Only Flags** (such as placement policies). Specialized GPU flags (e.g., multi-node NVLink topologies or custom GPU drivers) will be documented here as they are added.

#### 8.3.4 GKE & Advanced Orchestration Flags
*These flags unlock advanced GKE capabilities, including Kueue queue priorities, workload identities, network placements, and robust lifecycle failure policies.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `-q, --queue` | `string` | Name of the Kueue `LocalQueue` to submit the job to (Auto-discovered by default). |
| `--priority` | `string` | Priority class or level assigned to the job queue (e.g., `low`, `medium`, `high`). |
| `--gke-ttl-after-finished` | `string` | Time duration to retain the JobSet resources after completion (Default: `1h`). |
| `--grace-period` | `string` | Buffer period given to pods to save checkpoints before forced termination (Default: `30s`). |
| `--node-constraint` | `string` | Maps to Kubernetes node labels to target specific hardware instance types. |
| `--placement-policy` | `string` | Specifies a GCE Placement Policy name (e.g., `compact-placement`) to minimize latency. |
| `--restart-on-exit-codes` | `string` | Comma-separated list of retriable exit codes that bypass the main restart budget. |
| `--gke-scheduler` | `string` | Specific GKE scheduler selection (e.g., `gke.io/topology-aware-auto`). |
| `--image-pull-secret` | `string` | Secret name required to authenticate and pull images from private container registries. |
| `--service-account` | `string` | Kubernetes service account name used to provide fine-grained IAM roles to the job pods. |
| `--cpu-affinity` | `string` | CPU affinity rules (e.g., `'numa'`). |
| `--gke-disable-parallel-containers` | `bool` | Disable parallel containers for TPU v7/v7x on GKE. (Default: `false`) |

### 8.4 `list` Flags
*Use these flags to filter the list of jobs.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `--status` | `string` | Filter jobs by status (e.g., `Pending`, `Running`, `Succeeded`, `Failed`, `Suspended`). |
| `--name-contains` | `string` | Filter jobs by name containing the specified string. |

### 8.5 `logs` Flags
*Use these flags when fetching logs.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `-f, --follow` | `flag` | Stream logs continuously (like `tail -f`). |

## 9. Troubleshooting: ImagePullBackOff

If your job status remains `Pending` and the underlying pods show `ImagePullBackOff` or `ErrImagePull`, the GKE node pool service account may lack permission to read from the Artifact Registry repository.

A project administrator can grant the necessary access manually by running:

```bash
gcloud artifacts repositories add-iam-policy-binding <REPOSITORY_NAME> \
    --location <REGION/ZONE> \
    --project <PROJECT_ID> \
    --member "serviceAccount:<GKE_NODE_SERVICE_ACCOUNT>" \
    --role "roles/artifactregistry.reader"
```

> [!TIP]
> You can find the service account used by your node pool in the GKE console or by running `kubectl get nodes -o jsonpath='{.items[*].spec.providerID}'`.

## 10. Cleanup

To avoid incurring unnecessary costs, destroy the deployed GKE cluster and its resources:

```bash
./gcluster destroy <CLUSTER_NAME>
```

*You will be prompted to confirm the destruction (type `a` and press Enter).*
