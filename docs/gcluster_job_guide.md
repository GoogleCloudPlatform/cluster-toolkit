# Gcluster Job Submission Guide

This guide provides a step-by-step process to deploy a GKE cluster, submit a sample Python script as a job using `gcluster job submit` with on-the-fly image building via Crane, and then destroy the cluster.

## 1. Prerequisites (Automated by `gcluster job submit`)

`gcluster job submit` automates the check for required prerequisites. The tool will identify missing dependencies and print the necessary installation or remediation commands directly to your console for review and execution.

However, a few foundational components are still assumed or require your initial attention:

* **Go (1.20 or later):** Required for building the `gcluster` binary. The `make` command used in step 3 will handle Go module dependencies.
* **Google Cloud SDK (`gcloud`):** `gcluster` requires `gcloud` to be installed and available in your system's PATH to run prerequisite checks. If missing, checks will abort. Download and install it from [https://cloud.google.com/sdk/docs/install](https://cloud.google.com/sdk/docs/install).
* **A GCP Project:** You will need a Google Cloud Project with billing enabled and necessary APIs enabled. `gcluster` will check if a project is resolved from flags or persistent configuration.
* **Environment Variables (for On-the-Fly Builds):** If you use `--build-context` to build images on-the-fly, you must set:
  * `GCLUSTER_IMAGE_REPO`: The name of your Artifact Registry repository only (e.g., `gcluster-repo`). The tool will automatically construct the full path using the cluster's region and project ID.
  * `USER` or `USERNAME`: Used for unique image tagging (usually set automatically by your OS).

### Automated Prerequisite Checks Overview

When you run `gcluster job submit` (or other job commands), the tool will check for:

* **Google Cloud SDK**: Verifies `gcloud` is installed.
* **Gcloud Authentication**: Checks if authenticated and if Application Default Credentials (ADC) are valid.
* **`kubectl` Installation**: Checks if `kubectl` is installed.
* **GKE Auth Plugin**: Checks if `gke-gcloud-auth-plugin` is installed.
* **Container Credential Helper**: Checks if Docker is configured for GCR and Artifact Registry.
* **Artifact Registry API**: Checks if `artifactregistry.googleapis.com` is enabled.

If any non-foundational component is missing or unconfigured, `gcluster` will:
1. Accumulate the required commands to fix the environment (tailored to your OS where possible).
2. Write these commands to a setup script at `~/.gcluster/setup_prereqs.sh`.
3. Fail the command with a clear error message instructing you to inspect and run the script: `bash ~/.gcluster/setup_prereqs.sh`.

This ensures full transparency and gives you control over what gets installed on your system.

**State Persistence:** Successful checks are remembered in `~/.gcluster/job_prereq_state.json` to optimize subsequent runs. Checks are re-run if the state is older than 24 hours or if you switch projects.

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

Create a directory named `job_details` and place your application files inside it. This will serve as your build context for the job. Crane will package all files in this directory and add them to the image.

### `cluster-toolkit/job_details/app.py`

```python
# app.py
print("Hello from the gcluster job submit application!")
print("This is a sample application running on GKE.")
```

> [!NOTE]
> Crane does not execute a Dockerfile. It simply copies the contents of the build context directory into the image. If you need to install dependencies, make sure they are already present in your `--base-image`.

## 5. Deploy a GKE Cluster

For this example, we'll deploy a basic GKE cluster using the `hpc-gke.yaml` blueprint.

* **Ensure `gcloud` is configured with your project ID and a region/zone where GKE is available.**

    ```bash
        gcloud config set project <PROJECT_ID>
        gcloud config set compute/region <REGION/ZONE> # Or your preferred region
        ```

    * **Create the deployment directory:**

    ```bash
    ./gcluster create examples/hpc-gke.yaml --vars="project_id=<PROJECT_ID>,deployment_name=<CLUSTER_NAME>,region=<REGION/ZONE>,gcp_public_cidrs_access_enabled=false,authorized_cidr=$(curl -s ifconfig.me)/32"
    ```

    *Replace `<PROJECT_ID>` with your actual GCP Project ID.*

* **Deploy the GKE cluster:**

    ```bash
    ./gcluster deploy <CLUSTER_NAME>
    ```

    *This command will show a Terraform plan. You will be prompted to confirm the changes (type `a` and press Enter).*

    *This deployment process can take a significant amount of time (e.g., 10-20 minutes or more) as it provisions cloud resources.* Wait for the command to complete successfully.

## 6. `gcluster job submit` Command Reference

The `gcluster job submit` command deploys a container image as a job (Kubernetes JobSet) on a GKE cluster, integrated with Kueue. It can use pre-built images or build images on-the-fly using Crane.

### Supported Flags

Here are the flags currently supported by `gcluster job submit`:

* `-i, --image string`: Name of a pre-built container image to run. Must include the full path including registry (e.g., `<region>-docker.pkg.dev/my-project/my-repo/my-image:tag`). Use this if your image is already pushed to a registry.
* `-B, --base-image string`: Name of the base container image for Crane to build upon (e.g., `python:3.9-slim`). Required when using `--build-context` for an on-the-fly build.
* `-b, --build-context string`: Path to the build context directory for Crane (e.g., `./job_details`). Required with `--base-image`. Crane will package all files in this directory and append them as a new layer to the base image (it does not require or execute a Dockerfile).
* `-e, --command string`: Command to execute in the container (e.g., `'python app.py'`). This overrides the `CMD` instruction in your `Dockerfile`. (Required)
* `--compute-type string`: Type of compute to request (e.g., `'n2-standard-32'`, `'nvidia-l4'`, or shorthand strings for TPUs like `v6e-8`). (Required) The tool will resolve the machine type and calculate `num-nodes` and `topology` automatically if needed.
* `-o, --dry-run-out string`: Path to output the generated Kubernetes manifest instead of applying it directly to the cluster. Useful for inspection.
* `-c, --cluster string`: Name of the GKE cluster to deploy the job to. Optional if set in configuration.
* `-l, --location string`: Location (Zone or Region) of the GKE cluster. Optional if set in configuration.
* `-p, --project string`: Google Cloud Project ID. Optional if set in configuration.
* `-f, --platform string`: Target platform for the image build (e.g., `linux/amd64`, `linux/arm64`). Used with --base-image. (Default: `linux/amd64`)
* `-n, --name string`: Name of the job (JobSet) to create. This name will be used for Kubernetes resources. (Required)
* `--queue string`: Name of the Kueue LocalQueue to submit the job to. (Default: Auto-discovered from the cluster)
* `--num-slices int`: Number of independent groups/slices to use. (Default: `1`).
* `--num-nodes int`: Number of nodes to use per group/slice. (Default: `1`). Auto-calculated for TPUs based on topology.
* `--topology string`: TPU slice topology (e.g., `2x2x1`).
* `--restarts int`: Maximum number of restarts for the JobSet before failing. (Default: `1`)
* `--gke-ttl-after-finished string`: Time to retain the JobSet after it finishes (e.g. `5m`, `1h`, `3600`). (Default: `1h`)
* `--grace-period string`: Time to wait before forcefully terminating a pod (e.g. `30s`, `2m`). Gives the workload time to save checkpoints or clean up distributed state during job cancellation or hardware preemption events (like Spot VM evictions). (Default: `30s`)
* `--mount stringArray`: Mount a storage volume (format: `<src>:<dest>[:<mode>]`, mode can be `'ro'` or `'rw'`, default `'ro'`). Can be specified multiple times.
* `--pathways`: If present, gcluster will generate a manifest for a Pathways job.
* `--pathways-gcs-location string`: Please provide the GCS location to store Pathways artifacts. This flag is required when using --pathways.
* `--pathways-proxy-server-image string`: The image for the Pathways proxy server.
* `--pathways-server-image string`: The image for the Pathways server.
* `--pathways-worker-image string`: The image for the Pathways worker.
* `--pathways-headless`: If present, the user's workload container will not be deployed within the `pathways-head` job.
* `--pathways-elastic-slices int`: Configures the number of elastic slices.
* `--pathways-max-slice-restarts int`: Maximum times the workers in a slice can be restarted.
* `--pathways-proxy-args string`: Arbitrary additional command-line arguments for `pathways-proxy`.
* `--pathways-server-args string`: Arbitrary additional command-line arguments for `pathways-rm`.
* `--pathways-worker-args string`: Arbitrary additional command-line arguments for `pathways-worker`.
* `--pathways-colocated-python-sidecar-image string`: Image for an optional Python-based sidecar container.
* `--pathways-head-np string`: The node pool to use for the Pathways head job.

## 7. Submit the Sample Job with `gcluster job submit`

Now that the cluster is deployed and your application code is prepared, you can submit your sample Python script as a JobSet job. `gcluster job submit` will automatically build your container image using Crane and push it to Artifact Registry in your project.

> [!IMPORTANT]
> The image will be pushed to a regional Artifact Registry endpoint: `<region>-docker.pkg.dev/<project>/<GCLUSTER_IMAGE_REPO>/<user>-runner:<tag>`.

* You **must** set the `GCLUSTER_IMAGE_REPO` environment variable to specify the name of the Artifact Registry repository when using `--build-context` for on-the-fly builds (e.g., `export GCLUSTER_IMAGE_REPO=gcluster-repo`). The tool will automatically construct the full path using the cluster's region and project ID. The command will fail fast if this variable is not set. The repository **must exist** before submitting the job.
* You **must** have either `USER` or `USERNAME` environment variable set when using `--build-context`. `gcluster` uses this to ensure unique image tagging (e.g., `my-user-runner:tag`). The command will fail if both are missing.

### Unified Job Submission

By specifying the `--compute-type` flag, you can use the exact same command to deploy to a standard CPU cluster (using machine type like `n2-standard-32`) or an accelerated GPU/TPU cluster (using accelerator type like `nvidia-l4`). The orchestrator will calculate the necessary resource requests and limits based on the specified compute type.

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

* **Submit the Job:**

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

    *Replace `<PROJECT_ID>` with your actual GCP Project ID.*

    This command will:
    1. Verify/install the JobSet CRD on your cluster.
    2. Auto-discover the Kueue LocalQueue name from the cluster.
    3. Use the compute type installed on the cluster nodes and map the necessary resource requests.
    4. Build a container image from the job_details directory using python:3.9-slim as the base, and push it to Artifact Registry.
    5. Generate and apply an intelligently configured Kubernetes JobSet manifest to your cluster.

* **Example for Multi-Slice GPU Job:**
    If you want to run a job across multiple groups of GPU nodes (e.g., 2 groups of 4 nodes each), you can use `--num-slices` and `--num-nodes`:

    ```bash
    ./gcluster job submit \
      --project <PROJECT_ID> \
      --cluster <CLUSTER_NAME> \
      --location <REGION/ZONE> \
      --image us-docker.pkg.dev/my-project/my-repo/my-image:latest \
      --command "python train.py" \
      --name my-gpu-job \
      --compute-type l4-1 \
      --num-slices 2 \
      --num-nodes 4
    ```

    *This creates a JobSet with 2 replicas, each having 4 pods, totaling 8 nodes.*

### 7.1 Example: Submit Job with Persistent Storage (Mounting Bucket)

You can mount Cloud Storage buckets or host paths using the `--mount` flag. By default, mounts are read-only. You can specify read-write mode by appending `:rw` to the mount string:

```bash
./gcluster job submit \
  --project <PROJECT_ID> \
  --cluster <CLUSTER_NAME> \
  --location <REGION/ZONE> \
  --name my-storage-job \
  --command "python app.py" \
  --compute-type n2-standard-32 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --mount "gs://<YOUR_BUCKET_NAME>:/data:rw"
```

## 8. Verify the Job

Verify that the Kubernetes JobSet ran successfully on your GKE cluster.

* **Check Job Status:**
    You can check the status of your submitted job directly with `gcluster job list`:

    ```bash
    ./gcluster job list \
      --project <PROJECT_ID> \
      --cluster <CLUSTER_NAME> \
      --location <REGION/ZONE>
    ```

    Look for `my-python-app-job` with a `Succeeded` status.

* **Get Job Logs:**
    You can view the logs of your submitted job directly with `gcluster job logs`:

    ```bash
    ./gcluster job logs my-python-app-job \
      --project <PROJECT_ID> \
      --cluster <CLUSTER_NAME> \
      --location <REGION/ZONE>
    ```

    You should see the output:

    ```text
    Hello from the gcluster job submit application!
    This is a sample application running on GKE.
    ```

## 8. Verify Phase 2 Features (Advanced Scheduling & Lifecycle)

### 8.1 Run with Advanced Scheduling Flags

Try running a job with advanced scheduling options.

**Example 1: Target Specific Nodes (Node Constraint)**
Use `--node-constraint` to target specific hardware (e.g., C2 nodes). This maps to node labels in GKE and aligns with SLURM's `--constraint` flag for future compatibility.

```bash
./gcluster job submit \
  --project <PROJECT_ID> \
  --cluster <CLUSTER_NAME> \
  --location <REGION/ZONE> \
  --name my-machine-job \
  --command "python app.py" \
  --compute-type c2-standard-60 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --node-constraint "node.kubernetes.io/instance-type=c2-standard-60"
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
  --project <PROJECT_ID> \
  --cluster <CLUSTER_NAME> \
  --location <REGION/ZONE> \
  --name my-kueue-job \
  --command "python app.py" \
  --compute-type n2-standard-32 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --queue "my-local-queue"
```

(Note: You would need to ensure a Kueue `LocalQueue` named `my-local-queue` is configured on your cluster.)

### 8.2 Cancel Jobs

You can clean up specific job without destroying the entire cluster.

```bash
./gcluster job cancel my-python-app-job \
  --project <PROJECT_ID> \
  --cluster <CLUSTER_NAME> \
  --location <REGION/ZONE>
```

Verify it's gone by running `gcluster job list` again.

### 8.3 Job Retention (TTL)

By default, finished jobs are kept for 1 hour. You can change this using `--gke-ttl-after-finished` and pass flexible durations.

```bash
./gcluster job submit ... --gke-ttl-after-finished 10m # Keep for only 10 minutes
./gcluster job submit ... --gke-ttl-after-finished 2h  # Keep for 2 hours
```

### 8.4 Graceful Termination (Grace Period)

You can give your workloads a buffer period to save checkpoints or perform cleanups before they are forcefully killed using `--grace-period`.

```bash
./gcluster job submit ... --grace-period 2m # Allow 2 minutes for cleanup
```

### 8.4 Topology & Scheduler

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

## 11. Troubleshooting: ImagePullBackOff

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

## 12. Cleanup

To avoid incurring unnecessary costs, destroy the deployed GKE cluster and its resources:

```bash
./gcluster destroy <CLUSTER_NAME>
```

*You will be prompted to confirm the destruction (type `a` and press Enter).*
