# Gcluster Job Submission Guide

This guide provides a step-by-step process to deploy a GKE cluster, submit a sample Python script as a job using `gcluster job submit` with on-the-fly image building, and then destroy the cluster.

## 1. Prerequisites

Before you begin, ensure you have installed `gcluster` and set up your environment by following the instructions in the [cluster-toolkit README.md](../README.md#using-the-pre-built-bundle-recommended).

### Conditional Prerequisites (For On-the-Fly Builds)

If you use `--build-context` to build images on-the-fly, you must set:

* `GCLUSTER_IMAGE_REPO`: The name of your Artifact Registry repository only (e.g., `gcluster-repo`). The tool will automatically construct the full path using the cluster's region and project ID.
* `USER` or `USERNAME`: Used for unique image tagging (usually set automatically by your OS).

### Optional Environment Variables

* `GCLUSTER_REFRESH_CREDENTIALS`: Set to `1` to make every `gcluster job` command re-run `gcloud container clusters get-credentials` instead of reusing an existing, verified `kubeconfig` context for the cluster. See [Cluster connectivity and kubeconfig reuse](#101-cluster-connectivity-and-kubeconfig-reuse) for when this is useful.

> [!NOTE]
> ### Automated Prerequisite Checks Overview
>
> When you run `gcluster job submit` (or other job commands), the tool will check for:
>
> * **Google Cloud SDK**: Verifies `gcloud` is installed.
> * **Gcloud Authentication**: Checks if authenticated and if Application Default Credentials (ADC) are valid.
> * **`kubectl` Installation**: Checks if `kubectl` is installed.
> * **GKE Auth Plugin**: Checks if `gke-gcloud-auth-plugin` is installed.
> * **Artifact Registry API** (Crane builds only): Checks if `artifactregistry.googleapis.com` is enabled in the cluster's project.
> * **Container Credential Helper** (Crane builds only): Checks that Docker uses `gcloud` credentials for the Artifact Registry host the build pushes to (`<cluster-region>-docker.pkg.dev`).
>
> The two Crane-build checks only run when `gcluster` builds and pushes the image itself (`--base-image` with `--build-context`). A pre-built `--image` is pulled by the cluster nodes and needs neither.
>
> If any required dependencies are missing or unconfigured, `gcluster` will identify them and print the necessary installation or remediation commands directly to your console for review and execution
>
> Successful checks are remembered in `~/.gcluster/job_prereq_state.json` to optimize subsequent runs. Checks are re-run if the state is older than 24 hours or if you switch projects. The Docker credential check is not cached: it reads your local Docker config on every Crane build, because the registry depends on the cluster's region.
>
> **Bypassing Checks:** If you are running `gcluster` in a complex environment (such as an automated CI/CD pipeline without a human `gcloud` session, a system with custom Docker credential helpers, or an environment with strict least-privilege IAM limits), you can completely bypass these validations by appending the `--skip-prereqs` flag.

### Confirmation Prompts

Some `gcluster job` commands ask for confirmation before certain changes, for example before creating default Kueue queues or re-installing Kueue on the cluster. Each prompt reads one line from standard input:

* Enter (an empty line), `y` or `yes` means yes; `n` or `no` means no (case-insensitive). Any other answer is rejected, and the prompt asks again and reads the next line.
* If standard input is closed or ends before an answer (for example, `</dev/null`, or `<NUL` on Windows), the answer is no and the command stops with an error. If standard input stays open but sends nothing, the prompt waits.
* A heredoc or file can answer several prompts, one line each, but which prompts appear, and in what order, depends on the cluster's state and the `gcluster` version (`--skip-prereqs` skips only local prerequisite checks, not these cluster prompts). For example, when Kueue is outdated, a Kueue re-installation prompt appears before the queue prompt, so an answer meant for the queue prompt can approve the re-installation.

> [!WARNING]
> Any `y` or empty line on standard input can approve the Kueue re-installation prompt, which deletes all Kueue resources and all queued or suspended workloads in the cluster. Piping `yes` approves every prompt. For unattended runs, redirect standard input from `/dev/null` so unexpected prompts are declined, and pass `--queue` with an existing LocalQueue so the queue prompt does not appear.

### 1.1 Multi-Tier Checkpointing (MTC) Prerequisites

If you plan to use Multi-Tier Checkpointing (`--gke-mtc-enabled` flag), ensure that the Multi-Tier Checkpointing feature is enabled on your GKE cluster.

To use this feature, the cluster administrator must also deploy the `CheckpointConfiguration` Custom Resource that names the checkpoint Cloud Storage bucket, and grant the MTC driver access to that bucket (see [MTC blueprint configuration](gke-advanced-features.md#41-blueprint-configuration)). Once the cluster is configured, job submitters pass `--gke-mtc-enabled` with `--gke-mtc-ramdisk-dir <dir>` (for example `/tmp/mtc_checkpoints`) to set the local checkpoint directory. For more details on cluster configuration, see the [GKE Multi-Tier Checkpointing documentation](https://cloud.google.com/kubernetes-engine/docs/how-to/machine-learning/training/multi-tier-checkpointing).

## 2. Prepare Sample Application Code

Create a directory named `job_details` and place your application files inside it. This will serve as your build context for the job. The tool will package all files in this directory and add them to the image.

### `cluster-toolkit/job_details/app.py`

```python
# app.py
print("Hello from the gcluster job submit application!")
print("This is a sample application running on GKE.")
```

> [!NOTE]
> This on-the-fly image build does not execute a Dockerfile. It packages the files from your build context directory directly into the root directory (`/`) of the container image, honoring `.dockerignore` and skipping common VCS/build artifacts (such as `.git`, `.terraform`, `node_modules`, and `__pycache__`). If you need to install dependencies, make sure they are already present in your `--base-image`.

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

By specifying the `--compute-type` flag, you can use the exact same command to target a standard CPU cluster (using a full GCE machine type like `n2-standard-32`), an accelerated GPU cluster (using a GPU shorthand like `h100-80gb-8` or `l4-1`, a mapped GPU machine type like `a3-highgpu-8g`, or a GKE accelerator label like `nvidia-l4`), or a TPU cluster (using a shorthand string representing total chips/cores like `v6e-8`). GPU/TPU shorthands and machine types in `gcluster`'s built-in shorthand map resolve without a pre-existing node pool (so they also work on NAP clusters), whereas unmapped CPU machine types (e.g., `n2-standard-32`) and raw GKE accelerator labels (e.g., `nvidia-l4`) resolve against the cluster's existing node pools. The tool will automatically resolve the machine type, calculate `num-nodes`, and deduce the correct TPU topology if needed.

A TPU shorthand names a slice size, not a host shape. GKE can run the same slice on different machine types of a TPU family (an 8-chip `2x4` v5e slice is one `ct5lp-hightpu-8t` host or two `ct5lp-hightpu-4t` hosts), so `gcluster` uses the machine type of that family found in the cluster's node pools. `v5e-N` and `v5litepod-N` are equivalent. To pin a specific host shape, pass the full machine type (for example `--compute-type ct5lp-hightpu-4t`).

> [!TIP]
> **Simplify Commands with Configuration**: You can set these values once using the configuration command and omit them from subsequent commands:
>
> ```bash
> # Open system editor to edit defaults interactively
> ./gcluster job config set
>
> # Or set values individually
> ./gcluster job config set project <PROJECT_ID>
> ./gcluster job config set cluster <CLUSTER_NAME>
> ./gcluster job config set location <REGION/ZONE>
> ```
>
> To view your current configuration, run:
>
> ```bash
> ./gcluster job config show
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

### 4.4 Example: Submit Job with Persistent Storage

You can mount Cloud Storage buckets, Filestore instances, existing PVCs (e.g., for Lustre), or host paths using the `--mount` flag.

Mounts must use the format: `--mount "<src>;<dest>[;<mode>][;profile=<profile>][;options=<options>][;attributes=<k=v,...>]"`
* `mode` is optional and defaults to `ro` (read-only). To allow writes, append `;rw`.
* `options` is optional and allows passing custom mount options (currently only supported for GCS fuse volumes).
* `profile` is optional, GCS-only, and selects a [GKE Cloud Storage FUSE storage profile](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gcsfuse-profiles) (see below).
* `attributes` is optional, GCS-only and sets key-value pairs for CSI volume settings (`key=val,...`).
  * **Without `profile=`**: Applies standard [GCSFuse CSI volume attributes](https://docs.cloud.google.com/kubernetes-engine/docs/reference/cloud-storage-fuse-csi-driver/volume-attr) directly to the inline mount.
  * **With `profile=`**: Overrides default [storage profile StorageClass parameters](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gcsfuse-profiles#storageclass_configuration_reference) on the generated PVs.

**Supported volume sources (`<src>`):**
* **Cloud Storage**: `gs://<bucket-name>` (mounts via GCS Fused Driver)
* **Filestore**: `filestore://<instance-name-or-ip>/<share-name>` (auto-provisions PV and PVC)
* **Existing PVC**: `<pvc-name>` (the PersistentVolumeClaim must already exist in the target Kubernetes namespace)
* **Host Path**: `/host/path/on/node` (mounts a directory directly from the host node)

> [!NOTE]
> ### Shared VPC / Cross-Project / Cross-Region Filestore
>
> * **Cross-Project**: For Shared VPC or cross-project setups where the Filestore instance resides in a different project than your GKE cluster, you **must use the IP address** instead of the instance name. `gcluster` will automatically fall back to using the IP directly with a default capacity of `1Ti` (1024 GiB) if the API lookup fails (e.g. if the user lacks the **Filestore Viewer** (`roles/file.viewer`) or primitive **Viewer** (`roles/viewer`) IAM role in the Filestore's project) or returns no matches. A warning log will be printed when this fallback is triggered. Note that this default capacity is only a metadata placeholder to satisfy Kubernetes requirements and does not restrict the actual storage capacity available on your Filestore share.
> * **Cross-Region**: Mounting cross-region Filestores is supported as long as your VPC network has global routing enabled to allow cross-region NFS traffic. If instances with the same name exist in multiple regions, use the IP address to ensure the correct one is mounted.

**Examples:**

Mounting a GCS bucket (read-write):

```bash
./gcluster job submit \
  --name my-storage-job \
  --command "python app.py" \
  --compute-type n2-standard-32 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --service-account "workload-identity-k8s-sa" \
  --mount "gs://<YOUR_BUCKET_NAME>;/data;rw;options=logging:severity:info,enable-atomic-rename-object:true"
```

*(Note: Replace `<YOUR_BUCKET_NAME>` with the name of your own bucket. Use `--service-account <KSA_NAME>` to specify the Kubernetes Service Account configured with Workload Identity access to read/write the bucket).*

Mounting GCS buckets with storage profiles:

```bash
./gcluster job submit \
  --name my-training-job \
  --command "python app.py" \
  --compute-type n2-standard-32 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --service-account "workload-identity-k8s-sa" \
  --mount "gs://<YOUR_DATASET_BUCKET>/imagenet;/data;ro;profile=training" \
  --mount "gs://<YOUR_CHECKPOINT_BUCKET>/run-42;/checkpoints;rw;profile=checkpointing"
```

*(Note: Replace `<YOUR_DATASET_BUCKET>` and `<YOUR_CHECKPOINT_BUCKET>` with the names of your own buckets).*

Mounting an existing PVC named `lustre-pvc` (read-only):

```bash
./gcluster job submit \
  --name my-lustre-job \
  --command "python app.py" \
  --compute-type n2-standard-32 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --mount "lustre-pvc;/data"
```

#### GCS FUSE storage profiles (`profile=`)

Adding `profile=` (`training`, `checkpointing`, or `serving`) to a `gs://` mount makes `gcluster` generate a [GKE Cloud Storage FUSE storage profile](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gcsfuse-profiles) PersistentVolume and PersistentVolumeClaim alongside your JobSet. `gcluster job cancel` deletes them once no other workload uses them. For prerequisites, IAM and region requirements, the attributes reference, and how gateways are shared and cleaned up, see [Cloud Storage FUSE storage profiles](gke-advanced-features.md#5-cloud-storage-fuse-storage-profiles).

### 4.5 Example: Submit Job with Custom Environment Variables

For standard (non-Pathways) JobSet workloads, `gcluster job submit` automatically injects `WORKERS_0_HOSTNAME` and `MASTER_ADDR` into the workload container environment, both set to `<workload-name>-main-job-0-0.<workload-name>` (the DNS hostname of the rank-0 pod on the JobSet headless service) for distributed frameworks such as PyTorch `torchrun`.

You can pass additional custom environment variables—or override `WORKERS_0_HOSTNAME` / `MASTER_ADDR`—using the `--env` flag:

```bash
./gcluster job submit \
  --name my-env-job \
  --command "python app.py" \
  --compute-type n2-standard-32 \
  --base-image python:3.9-slim \
  --build-context job_details \
  --env "TRAINING_EPOCHS=10" \
  --env "DEBUG=true"
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

* **Inspect Cluster and Workload Health:**
    If you encounter scheduling delays, errors, or suspect resource exhaustion, you can run `gcluster job inspect` to capture a comprehensive diagnostic sweep of your cluster state and active workloads.

    ```bash
    ./gcluster job inspect
    ```

    To debug a specific workload, provide its name to fetch details like Kueue Workload status and JobSet configurations:

    ```bash
    ./gcluster job inspect --name my-python-app-job
    ```

    To view logs output in the console in addition to saving them to the diagnostic file, add the `--show` (or `-s`) flag:

    ```bash
    ./gcluster job inspect --name my-python-app-job --show
    ```

    The tool will create a timestamped log file `gcluster-inspect-<cluster>-<timestamp>.log` in your current working directory containing:

  * **Local Setup**: Gcloud version and active configuration.
  * **GKE Infra**: GKE cluster descriptions, node-pool listings, and metadata/resources ConfigMaps.
  * **Node Status**: Wide listing of nodes, along with Go-calculated counts of total and healthy nodes per node pool.
  * **Kueue / JobSet**: Configurations and logs for Kueue and JobSet controller managers.
  * **Slice Controller**: Slice controller deployment details and manager logs (if GKE Kueue dynamic slicing is active).
  * **Workloads**: Overview of all workloads in the cluster, and specific JobSet/Workload descriptors if a name is targeted.
  * **Console Links**: Direct links to GKE clusters, GKE workloads, IAM permissions, and Quota administration consoles.

> [!TIP]
> **Targeting a Specific Namespace**
> By default, `gcluster job` commands operate on the namespace detected from your active `kubeconfig` context (falling back to `default` if the context is found but its namespace is empty). If the context cannot be resolved, the command will abort with an error. To submit or manage jobs in a custom namespace, use the `--gke-namespace` flag:
>
> ```bash
> ./gcluster job submit ... --gke-namespace custom-namespace
> ./gcluster job list --gke-namespace custom-namespace
> ./gcluster job logs my-python-app-job --gke-namespace custom-namespace
> ./gcluster job cancel my-python-app-job --gke-namespace custom-namespace
> ./gcluster job inspect --gke-namespace custom-namespace
> ./gcluster job inspect --name my-python-app-job --gke-namespace custom-namespace
> ```

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

**Example 2: Target Multiple Node Pools (Fallback Scheduling)**
Use a pipe separator (`|`) in `--node-constraint` to specify multiple allowed values for standard constraints. This generates a `nodeAffinity` block instead of a strict `nodeSelector`, allowing the workload to schedule on any of the specified pools.

```bash
./gcluster job submit \
  --name my-fallback-job \
  --image <CONTAINER_IMAGE> \
  --command "python app.py" \
  --compute-type c2-standard-60 \
  --node-constraint "cloud.google.com/gke-nodepool=nodepool-a|nodepool-b"
```

> [!CAUTION]
> **TPU Topology Constraints (GKE Warden Limitation):**
>
> GKE clusters run a validating webhook (GKE Warden) that rejects any pod template where the affinity list for `cloud.google.com/gke-tpu-topology` contains **more than one unique value**.
>
> Consequently, passing multiple values separated by a pipe for TPU topology (e.g., `"2x2|4x4"`) is **strictly blocked** by `gcluster` client-side validation to avoid GKE admission rejection.
>
> **Best Practice (Queue-Based Fallback):**
> If you leave `--node-constraint` empty for TPU topology, GKE Kueue's `tryNextFlavor` cascading mechanism takes over automatically. Kueue will dynamically attempt to admit and schedule the job across all compatible resource pools (flavors) configured in the queue, mutating the pod template to target the single correct physical topology at admission time.

**Example 3: TPU Sub-Slicing**
If you want to schedule a job with a smaller TPU topology on a node pool with larger TPU slices (e.g., running a logical `2x2` workload on a physical `4x4` node pool):
* Specify the logical slice size in the `--topology` flag: `--topology 2x2` (or let it auto-discover from shorthand labels like `--compute-type v6e-4`)
* Specify the single physical topology size of the target node pool in `--node-constraint`: `--node-constraint "cloud.google.com/gke-tpu-topology=4x4"`

```bash
./gcluster job submit \
  --name my-subslicing-job \
  --image <CONTAINER_IMAGE> \
  --command "python app.py" \
  --compute-type v6e-4 \
  --node-constraint "cloud.google.com/gke-tpu-topology=4x4"
```

**Example 4: Use Placement Policy**
Use `--placement-policy` to name an existing GCE compact placement policy or workload policy. The job is pinned to nodes carrying that policy through the `cloud.google.com/placement-policy-name` node selector.

```bash
./gcluster job submit \
  ... \
  --name my-compact-job \
  --placement-policy "compact-placement"
```

*(Note: the policy must exist in the cluster's region and be attached to the target node pool. With `--gke-nap-provisioning`, GKE attaches it to the auto-created node pool instead; on machines that support compact placement the same name is also used as the `cloud.google.com/gke-placement-group` identifier.)*

A4X (`gb200-4`) and multi-host TPU 7x nodes require a `HIGH_THROUGHPUT` workload policy. When `--placement-policy` is omitted for these machine types, gcluster uses the policy attached to the matching node pool; under NAP it looks up, and if needed creates, the canonical policy (`gb200-4-1x72-placement-policy`, `tpu7x-<cores>-<topology>-placement-policy`) in the cluster's region.

**Example 5: Pod Failure Policy**
Use `--restart-on-exit-codes` to specify the allowlist of retriable container exit codes (`PodFailurePolicy` with `action: FailJob` on `NotIn`). Any non-zero exit code *not* in the list fails the JobSet immediately, whereas exit codes in the list allow the job to restart up to the `--restarts` budget.

```bash
./gcluster job submit \
  ... \
  --name my-robust-job \
  --restart-on-exit-codes 1,137,143
```

*(Note: Exit code `143` corresponds to `SIGTERM` ($128 + 15$). Workload pods interrupted by Spot or Kueue preemption receive SIGTERM and exit with code 143; adding 143 ensures preempted pods are restarted rather than failing the job immediately).*

**Example 6: Private Registry & Service Account**
Use `--image-pull-secret` and `--service-account` for secure jobs.

```bash
./gcluster job submit \
  ... \
  --name my-secure-job \
  --image-pull-secret "my-private-registry-secret" \
  --service-account "my-workload-sa"
```

**Example 7: Explicit Kueue Queue Selection**
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

### Example 8: Targeting Provisioning Models (Spot/Reservation) on GKE NAP Clusters

> [!NOTE]
> The `--gke-nap-provisioning` and `--gke-nap-reservation` flags are supported **only** on GKE clusters with Node Auto-Provisioning (NAP) enabled. They cannot be used on static GKE clusters where NAP is disabled, and doing so will result in an immediate pre-flight validation error.

Use the `--gke-nap-provisioning spot` option to run your workload on Spot VMs:

```bash
./gcluster job submit \
  --name my-spot-job \
  --image <CONTAINER_IMAGE> \
  --command "python app.py" \
  --compute-type v6e-4 \
  --gke-nap-provisioning spot
```

Use the `--gke-nap-provisioning reservation` option and target a GCE reservation via the `--gke-nap-reservation` flag:

```bash
./gcluster job submit \
  --name my-reservation-job \
  --image <CONTAINER_IMAGE> \
  --command "python app.py" \
  --compute-type v6e-4 \
  --gke-nap-provisioning reservation \
  --gke-nap-reservation my-tpu-reservation
```

### 6.3 Job Retention (TTL)

By default, finished jobs are kept for 1 hour. You can change this using `--gke-ttl-after-finished` and pass flexible durations.

```bash
./gcluster job submit ... --gke-ttl-after-finished 10m # Keep for only 10 minutes
./gcluster job submit ... --gke-ttl-after-finished 2h  # Keep for 2 hours
```

### 6.4 Graceful Termination & Signal Handling

You can give your workloads a buffer period to save checkpoints or perform cleanups before they are forcefully killed using `--grace-period`.

```bash
./gcluster job submit ... --grace-period 2m # Allow 2 minutes for cleanup
```

#### Signal Forwarding & Cleanup
When a workload pod is evicted, cancelled, or preempted (e.g. on Spot VMs or Kueue preemption), GKE sends `SIGTERM` to the container:
* `gcluster` runs workload commands in their own process group and forwards `SIGTERM` to child processes so active operations terminate promptly.
* Post-command cleanup commands in the user script (such as checkpoint flushes or artifact uploads to Cloud Storage) continue to run before the pod grace period expires.
* If a workload is interrupted by `SIGTERM`, `gcluster` ensures it exits with code `143` even if cleanup commands exit `0`, preventing evicted pods from falsely reporting success.

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
  --compute-type v6e-16 \
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

## 8. Advanced GKE infrastructure features

This section details how the `gcluster job submit` command orchestrates advanced Google Kubernetes Engine (GKE) hardware and scheduling capabilities, such as dynamic slice reconfiguration, the Pathways distributed AI framework, and GKE Node Auto-Provisioning (NAP).

> [!NOTE]
> For complete infrastructure formation guidelines, cluster blueprint Terraform variable configurations, and module requirements for deploying clusters that support these features, see the [guide to advanced GKE infrastructure features](gke-advanced-features.md).

---

### 8.1 Dynamic slicing (TPU v7x and future generations)

GKE Dynamic Slicing provides flexible TPU capacity scheduling by letting you logically group or slice physical nodes dynamically. In Cluster Toolkit and GKE, dynamic slicing is supported starting with TPU v7x (Ironwood) and future TPU generations (earlier generations like TPU v4, TPU v5e, TPU v5p, and TPU v6e require static slice topologies). Dynamic slicing supports both superslicing (aggregating multiple physical cubes into larger topologies) and subslicing (partitioning a single cube into smaller fractional topologies) dynamically at job submission time.

#### Capabilities and scheduling benefits

Dynamic slicing provides the following scheduling capabilities:

* **Dynamic superslicing:** Aggregate multiple physical TPU v7x cubes together into a larger logical slice (such as combining multiple `4x4x4` cubes into `4x4x8` or `4x4x16` topologies) dynamically for large-scale distributed training.
* **Dynamic subslicing:** Partition a single physical TPU cube into smaller fractional topologies (such as slicing a `4x4x4` cube into `2x2x4` or `2x4x4` sub-slices) dynamically, enabling efficient bin-packing and co-tenancy for smaller workloads.
* **Latency optimization:** Kueue Topology-Aware Scheduling (TAS) places TPU pods with minimal network hop latency across the physical TPU interconnect mesh, maximizing distributed training throughput.
* **Multi-slice synchronization:** For multi-slice jobs (`--num-slices > 1`), Cluster Toolkit coordinates slice-level reservations to ensure that all slices are acquired concurrently, preventing mismatched scaling.
* **Internal scheduling annotations:** Cluster Toolkit automatically translates high-level TPU requests into partition-level requirements (`cloud.google.com/gke-tpu-partition-TOPOLOGY-id`) and switches between single-slice (`kueue.x-k8s.io/podset-required-topology`) and multi-slice (`kueue.x-k8s.io/podset-slice-required-topology`) admission annotation keys.

#### Example CLI command

To submit a dynamic slicing workload targeting TPU v7x nodes, enter the following command:

```shell
./gcluster job submit \
  --name my-dynamic-slice-job \
  --image <CONTAINER_IMAGE> \
  --command "python train.py" \
  --compute-type tpu7x \
  --topology 4x4x4
```

#### GKE documentation reference

* [TPU Dynamic Slicing on GKE Concepts](https://cloud.google.com/kubernetes-engine/docs/concepts/dynamic-slicing)
* [Scheduling Dynamic Slices with Kueue and TAS on GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/use-gke-dynamic-slicing)

---

### 8.2 Pathways distributed AI framework

Pathways is a distributed execution framework for large-scale multi-slice machine learning workloads. The `gcluster job submit` command provides end-to-end integration for deploying Pathways-enabled jobs on GKE.

#### Capabilities and workload orchestration roles

Pathways integration provides the following orchestration capabilities:

* **Multi-role JobSet orchestration:** When you specify the `--pathways` flag, Cluster Toolkit generates and coordinates a multi-role JobSet comprising the Resource Manager (`pathways-rm`), Proxy server (`pathways-proxy`), and JAX worker pods.
* **Kueue resource management:** Cluster Toolkit automatically maps multi-slice resource limits and unifies Kueue ClusterQueue resource groups so that coordinator and worker pods schedule without resource deadlocks.
* **Headless execution mode:** With the `--pathways-headless` flag, Cluster Toolkit provisions the Pathways server infrastructure without requiring a client workload container, letting external clients (such as local notebooks) connect to the cluster.
* **Completion and restart semantics:** In non-headless mode, the JobSet's `successPolicy` tracks `pathways-head`: when `workload-container` exits `0`, the entire JobSet is marked `Completed` and the worker pods are torn down; when it exits non-zero (including `143` on `SIGTERM`), the JobSet `failurePolicy` restarts the job up to `--restarts` times. Ensure your training command exits non-zero on fatal errors (for example, if a framework catches checkpoint or data-loader exceptions and exits `0` early, the JobSet will complete as `Succeeded` instead of restarting).

#### Example CLI commands

To submit a standard multi-slice Pathways training workload, enter the following command:

```shell
./gcluster job submit \
  --name my-pathways-job \
  --image <CONTAINER_IMAGE> \
  --command "python train_pathways.py" \
  --compute-type v6e-16 \
  --pathways \
  --pathways-gcs-location gs://BUCKET_NAME/pathways-artifacts
```

To submit a headless Pathways server cluster, enter the following command:

```shell
./gcluster job submit \
  --name my-pathways-headless \
  --compute-type v6e-16 \
  --pathways \
  --pathways-gcs-location gs://BUCKET_NAME/pathways-artifacts \
  --pathways-headless
```

---

### 8.3 Node auto-provisioning (NAP) and compute consumption

GKE Node Auto-Provisioning (NAP) automatically provisions and manages node pools based on unschedulable pod requirements.

> [!NOTE]
> When running on a NAP-enabled cluster, standard job submissions without `--gke-nap-*` flags automatically trigger on-demand node pool creation if existing nodes lack sufficient capacity. The `--gke-nap-provisioning` and `--gke-nap-reservation` flags are only required when you want to target **Spot VMs** or specific **Compute Engine reservations**.
>
> Cluster Toolkit currently supports **On-Demand**, **Spot**, and **Reservation** models with Node Auto-Provisioning. Dynamic Workload Scheduler (DWS) Flex-Start and Queued Provisioning are supported via static cluster blueprints.

#### Capabilities, scheduling, and pre-flight verification

NAP integration provides the following capabilities:

* **Consumption targeting:** The `--gke-nap-provisioning` flag lets you target `spot`, `on-demand`, or `reservation` compute models.
* **Reservation integration:** The `--gke-nap-reservation RESERVATION_NAME` flag injects reservation affinity selectors, letting NAP spawn node pools directly in specific Compute Engine reservations.
* **Pre-flight limit verification:** When `--gke-nap-provisioning` is passed, Cluster Toolkit validates before submission that the requested accelerator resource type (or the `cpu` limit for CPU machine types) has a non-zero maximum in the cluster's NAP resource limits.

#### Example CLI commands

To target Spot VMs, enter the following command:

```shell
./gcluster job submit \
  --name my-nap-spot-job \
  --image <CONTAINER_IMAGE> \
  --command "python app.py" \
  --compute-type v6e-4 \
  --gke-nap-provisioning spot
```

To target a Compute Engine reservation, enter the following command:

```shell
./gcluster job submit \
  --name my-nap-reservation-job \
  --image <CONTAINER_IMAGE> \
  --command "python app.py" \
  --compute-type v6e-4 \
  --gke-nap-provisioning reservation \
  --gke-nap-reservation RESERVATION_NAME
```

---

### 8.4 Custom Kubernetes manifest templates

GCluster uses embedded Go templates to generate the Kubernetes manifests (JobSets, Kueue configurations, etc.) deployed to your GKE cluster. If you need to deeply customize the generated manifests beyond what the CLI flags provide (e.g., adding custom sidecars, specific annotations, or modifying the core structure), you can override these embedded templates with your own local copies.

#### 1. Extract the default templates

First, use the `gke-template-extract` command to extract the embedded default templates into a local directory so you have a starting point:

```bash
./gcluster job gke-template-extract --output-dir=/path/to/my-templates
```

*(Use `--overwrite` if the directory already contains files you want to replace).*

Extracted templates match the gcluster version that extracted them. After upgrading gcluster, extract them again and re-apply your changes, because newer versions can move logic into the templates. For example, `pathways_jobset.tmpl` now defines the Pathways Multi-Tier Checkpointing and colocated-sidecar volumes itself, so an older copy of that file omits them.

#### 2. Modify the templates

Open the extracted `.tmpl` files in your preferred editor. These are standard Go text templates (`text/template` and `yamltemplate`). You can inject any static Kubernetes YAML or modify the existing template variables.

> [!NOTE]
> `gcluster job logs --main-only` lets `kubectl` choose which container to read: the one named by the pod-template annotation `kubectl.kubernetes.io/default-container`, otherwise the first entry in `containers` (kubectl 1.24 or later; older versions require the annotation when there are several containers). Set this annotation if your template lists a regular (non-native) sidecar before your workload container.

#### 3. Use your custom templates

When submitting your job, you can tell `gcluster` to use your custom templates instead of the embedded ones.

Pass the `--gke-custom-templates-path` flag to the `submit` command:

```bash
./gcluster job submit ... --gke-custom-templates-path=/path/to/my-templates
```

> [!TIP]
> If a template file (e.g., `jobset.tmpl`) is not found in your custom directory, `gcluster` will automatically fall back to using its embedded default version for that specific file.

## 9. `gcluster job` Command Reference

### 9.1 Common Flags
*These flags are common to almost all `gcluster job` subcommands (except `config`). `project`, `cluster`, and `location` can be set as defaults via `config set`.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `-c, --cluster` | `string` | Name of the target GKE cluster. |
| `-l, --location` | `string` | Google Cloud location (Zone or Region) of the GKE cluster. |
| `-p, --project` | `string` | Google Cloud Project ID. |
| `--gke-namespace` | `string` | Target GKE namespace for the operation. Supported across all job commands. If omitted, automatic detection is used. |
| `--skip-prereqs` | `flag` | Bypasses local workstation pre-flight environment checks (e.g. gcloud SDK, IAM checks, Docker config). |

### 9.2 Configuration Commands
*Use these commands to manage persistent defaults for your job submissions, avoiding the need to pass common flags repeatedly.*

#### `gcluster job config set [key] [value]`
Modifies configuration defaults. If run without any arguments (`./gcluster job config set`), it automatically opens your default system text editor (`$EDITOR` or `nano`) to edit the configuration file interactively.

* **Supported Keys:**
  * `project`: Google Cloud Project ID
  * `cluster`: GKE Cluster Name
  * `location`: GKE Cluster Location (region or zone)

**Examples:**

```bash
# Interactive editor mode:
./gcluster job config set

# Set specific key/value pairs:
./gcluster job config set project my-awesome-project
./gcluster job config set cluster my-cluster
./gcluster job config set location us-central1-a
```

#### `gcluster job config show`
Prints the current saved configuration defaults and configuration file path.

**Example:**

```bash
./gcluster job config show
```

### 9.3 `submit` Flags
The `gcluster job submit` command deploys a container image as a job (Kubernetes JobSet) on a GKE cluster, integrated with Kueue for advanced queuing. It can use pre-built images or build images on-the-fly without a local Docker daemon (powered internally by the [Crane](https://github.com/google/go-containerregistry/blob/main/cmd/crane/README.md) container utility).

#### 9.3.1 General Flags
*These flags control core workload execution, identity, and basic build context options regardless of the underlying hardware paradigm.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `-n, --name` | `string` | Name of the job (JobSet) to create. Used for Kubernetes resources. Maximum of 28 characters for standard workloads, or 22 characters when `--pathways` is enabled. *(Required)* |
| `-e, --command` | `string` | Command to execute inside the container (e.g., `'python app.py'`). *(Required except when `--pathways-headless` is set)* |
| `--compute-type` | `string` | The hardware target for the job. GPU/TPU shorthands (e.g., `h100-80gb-8`, `l4-1`, `v6e-8`) and machine types from `gcluster`'s built-in shorthand map (e.g., `a3-highgpu-8g`) resolve without a matching node pool, so they also work on NAP clusters. Any other GCE machine type (e.g., `n2-standard-32`) or a GKE accelerator label (e.g., `nvidia-l4`) is resolved against the cluster's existing node pools. *(Required)* The tool will automatically resolve the machine type, calculate `num-nodes`, and deduce the correct TPU topology if needed. |
| `-i, --image` | `string` | Full registry path of a pre-built container image to run. Either `--image`, **or** both `--base-image` and `--build-context`, is required and the two forms are mutually exclusive (except when `--pathways-headless` is set, where no image is needed). |
| `-B, --base-image` | `string` | Name of the base container image to build upon (e.g., `python:3.9-slim`). Requires `--build-context`; mutually exclusive with `--image`. |
| `-b, --build-context` | `string` | Path to the local build context directory for on-the-fly image builds. Requires `--base-image`; mutually exclusive with `--image`. |
| `-f, --platform` | `string` | Target platform architecture for the image build (Default: `linux/amd64`). |
| `-o, --dry-run-out` | `string` | Local file path to save the generated Kubernetes manifest instead of applying it (must specify a file path, not a directory). |
| `--num-slices` | `int` | Number of independent groups/slices to use (Default: `1`). |
| `--num-nodes` | `int` | Number of nodes to use per group/slice (Default: `1`). Auto-calculated for TPUs based on topology. |
| `--node-constraint` | `string` | Maps to Kubernetes node labels to target specific hardware instance types. Supports pipe separator (`|`) for multiple values. |
| `--restarts` | `int` | Maximum number of restarts allowed for the JobSet before marked as failed (Default: `1`). |
| `--mount` | `stringArray` | Mount storage volumes, buckets, filestore instances, or PVCs using the `<src>;<dest>[;<mode>][;profile=<profile>][;options=<options>][;attributes=<k=v,...>]` format. Examples of `<src>`: `gs://my-bucket`, `filestore://my-instance/share`, `my-pvc` (for Lustre/etc), or `/host/path`. `profile=` (`training`, `checkpointing`, `serving`) is GCS-only and provisions a shared GCS FUSE storage-profile PV/PVC gateway instead of an inline CSI volume. |
| `--env` | `stringArray` | Custom environment variables to pass exclusively to the user's workload container in KEY=VALUE format (e.g. `--env KEY=VALUE`). Applies to both standard and Pathways workloads and can be specified multiple times. For standard JobSet workloads, `WORKERS_0_HOSTNAME` and `MASTER_ADDR` are preset to `<name>-main-job-0-0.<name>` unless overridden via `--env`. |
| `--await-job-completion` | `bool` | If true, the CLI waits for the job to complete before exiting. |
| `--timeout` | `string` | Time to wait for job completion (e.g., `1h`, `10m`). Used with `--await-job-completion`. |
| `--verbose` | `bool` | Enable verbose logging for the workload. |

*(Note: `--cluster`, `--location`, and `--project` are also supported as common flags, see 9.1)*

#### 9.3.2 TPU Related Flags
*Use these flags to orchestrate specialized TPU multi-slice topologies or leverage the Pathways distributed execution framework.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `--topology` | `string` | TPU slice topology (e.g., `2x2x1`, `2x4`, `2x4x4`). Applies to TPU workloads only (rejected for GPU/CPU jobs). **Required for TPU 7x.** For other TPU types, when omitted, `gcluster` resolves it from a slice shorthand (e.g., `v6e-16` -> `4x4`) or auto-discovers it from matching TPU node pools on the cluster. |
| `--pathways` | `flag` | If present, generates a manifest tailored for a Pathways distributed job paradigm. |
| `--pathways-gcs-location` | `string` | GCS bucket location to store Pathways artifacts. *(Required when --pathways is set)* |
| `--pathways-proxy-server-image` | `string` | Container image for the Pathways proxy server. |
| `--pathways-server-image` | `string` | Container image for the Pathways resource manager / server. |
| `--pathways-worker-image` | `string` | Container image for the Pathways workers. |
| `--pathways-headless` | `flag` | If present, the user workload container will not be deployed in the `pathways-head` job. |
| `--pathways-elastic-slices` | `int` | Configures the number of elastic slices for resilient training scaling. |
| `--pathways-max-slice-restarts` | `int` | Maximum number of times the workers within a single slice can be restarted (Default: `0`, or `4000` when `--pathways-elastic-slices > 0` or `--pathways-headless` is set). Maps to `backoffLimitPerIndex: N` and `backoffLimit: N * nodesPerSlice` on the Pathways worker Job. |
| `--pathways-proxy-args` | `string` | Arbitrary additional command-line arguments passed to `pathways-proxy`. |
| `--pathways-server-args` | `string` | Arbitrary additional command-line arguments passed to `pathways-rm`. |
| `--pathways-worker-args` | `string` | Arbitrary additional command-line arguments passed to `pathways-worker`. |
| `--pathways-proxy-env` | `stringArray` | Custom environment variables injected specifically into the Pathways proxy container (KEY=VALUE). |
| `--pathways-server-env` | `stringArray` | Custom environment variables injected specifically into the Pathways server container (KEY=VALUE). |
| `--pathways-worker-env` | `stringArray` | Custom environment variables injected specifically into the Pathways worker containers (KEY=VALUE). |
| `--pathways-colocated-python-sidecar-image` | `string` | Image for an optional Python-based sidecar container running alongside workers. |
| `--pathways-head-np` | `string` | The node pool name to target for the Pathways head job. |

#### 9.3.3 GPU Related Flags
*Use these flags to tune specialized multi-GPU topologies and related node parameters.*

> [!NOTE]
> Currently, GPU workloads leverage the standard **General Flags** (such as passing GPU shorthands like `--compute-type h100-80gb-8` or `--compute-type l4-1`, mapped machine types like `--compute-type a3-highgpu-8g`, or an accelerator label matching an existing node pool like `--compute-type nvidia-l4`) and **GKE Only Flags** (such as placement policies). Specialized GPU flags (e.g., multi-node NVLink topologies or custom GPU drivers) will be documented here as they are added.

#### 9.3.4 GKE & Advanced Orchestration Flags
*These flags unlock advanced GKE capabilities, including Kueue queue priorities, workload identities, network placements, and robust lifecycle failure policies.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `-q, --queue` | `string` | Name of the Kueue `LocalQueue` to submit the job to (Auto-discovered by default). |
| `--priority` | `string` | Priority class name assigned to the job queue (supports default classes like `low`, `medium`, `high`, or any custom PriorityClass defined in the cluster). If empty, the cluster's default priority class will be used. |
| `--grace-period` | `string` | Buffer period given to pods to save checkpoints before forced termination (Default: `30s`). |
| `--gke-mtc-enabled` | `flag` | If present, enables Multi-Tier Checkpointing (MTC) for the workload. |
| `--gke-mtc-ramdisk-dir` | `string` | The ramdisk directory path for local checkpoints in MTC (required when `--gke-mtc-enabled` is set). |
| `--gke-ttl-after-finished` | `string` | Time duration to retain the JobSet resources after completion (Default: `1h`). |
| `--placement-policy` | `string` | Name of an existing GCE compact placement or workload policy; the job is pinned to nodes carrying it via `cloud.google.com/placement-policy-name`. Auto-resolved for A4X and multi-host TPU 7x when omitted. |
| `--restart-on-exit-codes` | `string` | Comma-separated allowlist of retriable container exit codes (`1..255`). Exit codes in this list allow the job to restart up to `--restarts`, while any other non-zero exit code fails the job immediately (`action: FailJob` on `NotIn`). |
| `--gke-scheduler` | `string` | Specific GKE scheduler selection (e.g., `gke.io/topology-aware-auto`). |
| `--image-pull-secret` | `string` | Secret name required to authenticate and pull images from private container registries. |
| `--service-account` | `string` | Kubernetes service account name used to provide fine-grained IAM roles to the job pods. |
| `--cpu-affinity` | `string` | CPU affinity rules (e.g., `'numa'`). |
| `--gke-disable-parallel-containers` | `bool` | Disable parallel containers for TPU v7/v7x on GKE. (Default: `false`) |
| `--gke-nap-provisioning` | `string` | Compute provisioning model for GKE NAP. Allowed values: `on-demand`, `spot`, `reservation`. |
| `--gke-nap-reservation` | `string` | Name of the Google Cloud Reservation for GKE NAP (required if `--gke-nap-provisioning=reservation`). |
| `--gke-custom-templates-path` | `string` | Path to a local directory containing custom GKE manifest template overrides. |

### 9.4 `list` Flags
*Use these flags to filter the list of jobs.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `--status` | `string` | Filter jobs by status (e.g., `Pending`, `Running`, `Succeeded`, `Failed`, `Suspended`). |
| `--name-contains` | `string` | Filter jobs by name containing the specified string. |

### 9.5 `logs` Flags
*Use these flags when fetching logs.*

| Flag | Type | Description |
| :--- | :--- | :--- |
| `-f, --follow` | `flag` | Stream logs continuously (like `tail -f`). |
| `--main-only` | `bool` | Fetch logs only from the rank-0 pod of the JobSet's first replicated job (`main-job`, or `pathways-head` for Pathways), using that pod's default container. |

> [!NOTE]
> **Smart Logging Defaults**: If a job has more than 5 pods, `gcluster` dynamically defaults to `--main-only=true` to prevent terminal spam from duplicate worker rank logs. For jobs with up to 10 pods, you can override this to stream logs from all pods by explicitly passing `--main-only=false`; for jobs with more than 10 pods, `gcluster` prints a link to view all-pod logs in the Google Cloud Console.

## 10. Troubleshooting

### 10.1 Cluster connectivity and kubeconfig reuse

Every `gcluster job` command first makes `kubectl` target your cluster. A GKE control plane can be reached through two kinds of endpoints, and which one works depends on *where you run `gcluster` from*:

| Endpoint | Typical failure when it is the wrong one for your network |
| :--- | :--- |
| **IP endpoint** (`https://<public-or-private-ip>`) — subject to [Master Authorized Networks](https://cloud.google.com/kubernetes-engine/docs/how-to/authorized-networks) | `dial tcp <ip>:443: i/o timeout` (your client IP is not allow-listed, or has no route to the control plane), or `502 Bad Gateway` / `504 Gateway Timeout` when a corporate forward proxy answers on the control plane's behalf |
| **DNS endpoint** (`https://gke-<id>.<region>.gke.goog`) — IAM-gated, bypasses authorized networks | `431 Request Header Fields Too Large` (some corporate proxies inflate request headers) |

`gcluster` handles this as follows:

1. **Reuse first.** If your `kubeconfig` already contains a context for the cluster (`gke_<project>_<location>_<cluster>`) that *verifiably* belongs to it — its server is one of the cluster's endpoints, its pinned CA matches the cluster CA (IP endpoint only), and it authenticates through `gke-gcloud-auth-plugin` — `gcluster` runs a short (5s) connectivity probe against that context and, on success, **uses it as-is** without calling `gcloud container clusters get-credentials`. Your namespace and endpoint choice are preserved. You will see:

   ```text
   Reusing existing kubeconfig context 'gke_my-project_us-central1_my-cluster' (DNS endpoint).
   ```

2. **Otherwise fetch credentials and verify.** If there is no reusable context (or the probe shows the existing endpoint is unreachable from your machine), `gcluster` runs `get-credentials`, probes again, and if the probe fails with a *connectivity* error it automatically retries with the other endpoint when the cluster offers one:

   ```text
   WARNING: Cluster 'my-cluster' is unreachable via its IP endpoint (dial tcp 35.1.2.3:443: i/o timeout). Retrying with the DNS endpoint...
   WARNING: Connected to cluster 'my-cluster' via its DNS endpoint after the IP endpoint was unreachable. kubeconfig context 'gke_my-project_us-central1_my-cluster' now points at the DNS endpoint.
   ```

   Authentication errors (`Unauthorized`, `gke-gcloud-auth-plugin` failures) are **not** retried against another endpoint because a different endpoint cannot fix credentials; `gcluster` fails immediately and tells you to run `gcloud auth application-default login`.

After this step, every `kubectl` call and Kubernetes API request that `gcluster` makes passes the chosen context explicitly (`kubectl --context <context> ...`). Switching `current-context` in another terminal, or another tool running `get-credentials` while `gcluster` is running, does not redirect the job to a different cluster. When an existing context is reused, `gcluster` does not modify your kubeconfig at all; `current-context` only changes when credentials have to be fetched, because `gcloud container clusters get-credentials` sets it.

> [!TIP]
> **Forcing a credential refresh.** Set `GCLUSTER_REFRESH_CREDENTIALS=1` to skip step 1 and always re-run `get-credentials` (step 2 still applies). Use this if you suspect the reused context is wrong in a way the checks above cannot detect — for example, after deliberately editing the entry by hand, or when sharing a `kubeconfig` between tools that expect a specific endpoint:
>
> ```bash
> GCLUSTER_REFRESH_CREDENTIALS=1 ./gcluster job submit ...
> ```

If **both** endpoints are unreachable, the error lists what was attempted for each. Typical fixes are adding your egress IP to the cluster's authorized networks, enabling external traffic on the DNS endpoint (`gcloud container clusters update <CLUSTER> --location=<LOCATION> --enable-dns-access`), or running `gcluster` from a network that can reach the control plane.

### 10.2 ImagePullBackOff

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

## 11. Cleanup

To avoid incurring unnecessary costs, destroy the deployed GKE cluster and its resources:

```bash
./gcluster destroy <CLUSTER_NAME>
```

*You will be prompted to confirm the destruction (type `a` and press Enter).*
