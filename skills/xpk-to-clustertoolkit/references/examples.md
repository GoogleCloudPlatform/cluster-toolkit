<!--
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# End-to-End Migration Examples: XPK to Cluster Toolkit

This document provides complete, verified, copy-pasteable migration recipes for common AI/ML training workloads derived from production upstream MaxText recipes and Cluster Toolkit blueprints.

> [!NOTE]
> Recipes §1–§5 are derived from proposed upstream MaxText PRs (#5176, #5177, #5183, #5184, #5187) and merged Cluster Toolkit features (unmerged in MaxText at time of writing).
> Upstream recipes utilize the `gcluster job config set` preamble idiom:
> ```bash
> gcluster job config set project "${PROJECT_ID}"
> gcluster job config set cluster "${GKE_CLUSTER}"
> gcluster job config set location "${LOCATION}"
> ```
> This sets the default CLI context so subsequent `gcluster job submit`, `gcluster job list`, and `gcluster job logs` commands can omit repetitive connection flags.
> For container builds using Crane with `--base-image` and `--build-context`, set `export GCLUSTER_IMAGE_REPO=maxtext-images` (or your target registry repository). Workload pods can be inspected using the label selector: `kubectl get pods -l gcluster.google.com/workload=<JOB_NAME>`.

---

## 1. MaxText Supervised Fine-Tuning (SFT) & LoRA Post-Training

Migrating core post-training workloads from XPK to Cluster Toolkit (derived from upstream MaxText PR #5176 & PR #5177).

### 1A. SFT with Multi-Controller JAX (McJAX) on TPU v5p

#### Input XPK Command

```bash
xpk workload create \
  --cluster="${GKE_CLUSTER}" \
  --project="${PROJECT_ID}" \
  --zone="${LOCATION}" \
  --workload="${RUN_NAME}" \
  --tpu-type=v5p-128 \
  --num-slices=1 \
  --docker-image="${DOCKER_IMAGE}" \
  --command="python3 -m maxtext.trainers.post_train.sft.train_sft run_name=${RUN_NAME} base_output_directory=${BASE_OUTPUT_DIRECTORY} model_name=${MODEL} steps=${STEPS}"
```

#### Migrated Cluster Toolkit Command

```bash
gcluster job submit \
  --name "${RUN_NAME}" \
  --cluster "${GKE_CLUSTER}" \
  --project "${PROJECT_ID}" \
  --location "${LOCATION}" \
  --num-slices 1 \
  --image "${DOCKER_IMAGE}" \
  --command "python3 -m maxtext.trainers.post_train.sft.train_sft run_name=${RUN_NAME} base_output_directory=${BASE_OUTPUT_DIRECTORY} model_name=${MODEL} steps=${STEPS}" \
  --compute-type ct5p-hightpu-4t \
  --topology 4x4x4
```

*(Note: `--num-nodes` is omitted automatically for TPU jobs in Cluster Toolkit; node count is calculated directly from the mesh topology).*

### 1B. SFT with Pathways on TPU v5p

#### Input XPK Command

```bash
xpk workload create-pathways \
  --cluster="${GKE_CLUSTER}" \
  --project="${PROJECT_ID}" \
  --zone="${LOCATION}" \
  --workload="${RUN_NAME}" \
  --tpu-type=v5p-128 \
  --num-slices=1 \
  --docker-image="${DOCKER_IMAGE}" \
  --pathways-gcs-location="${BASE_OUTPUT_DIRECTORY}" \
  --command="python3 -m maxtext.trainers.post_train.sft.train_sft run_name=${RUN_NAME} base_output_directory=${BASE_OUTPUT_DIRECTORY} model_name=${MODEL} steps=${STEPS} enable_single_controller=True"
```

#### Migrated Cluster Toolkit Command

```bash
gcluster job submit --pathways \
  --name "${RUN_NAME}" \
  --cluster "${GKE_CLUSTER}" \
  --project "${PROJECT_ID}" \
  --location "${LOCATION}" \
  --num-slices 1 \
  --image "${DOCKER_IMAGE}" \
  --command "python3 -m maxtext.trainers.post_train.sft.train_sft run_name=${RUN_NAME} base_output_directory=${BASE_OUTPUT_DIRECTORY} model_name=${MODEL} steps=${STEPS} enable_single_controller=True" \
  --pathways-gcs-location "${BASE_OUTPUT_DIRECTORY}" \
  --compute-type ct5p-hightpu-4t \
  --topology 4x4x4
```

> [!NOTE]
> When submitting with `--pathways`, Cluster Toolkit automatically configures the Pathways proxy and coordination environment. You do not need to manually prefix commands with `JAX_PLATFORMS=proxy JAX_BACKEND_TARGET=grpc://127.0.0.1:29000`.
>
> **Workload Name Constraint**: Pathways workload names cannot exceed 22 characters due to the 63-byte Kubernetes label limit on coordinator addresses (`<name>-pathways-head-0-0.<name>`).

---

## 2. MaxText Elastic Training with Pathways

Migrating multi-slice resilient workloads with dynamic worker failure recovery (derived from upstream MaxText PR #5184).

### Input XPK Command

```bash
xpk workload create-pathways \
  --cluster="${GKE_CLUSTER}" \
  --project="${PROJECT_ID}" \
  --zone="${LOCATION}" \
  --workload="${RUN_NAME}" \
  --tpu-type=v5litepod-16 \
  --num-slices=3 \
  --docker-image="${DOCKER_IMAGE}" \
  --elastic-slices=1 \
  --max-slice-restarts=10 \
  --pathways-gcs-location="${BASE_OUTPUT_DIRECTORY}" \
  --command="python3 -m maxtext.trainers.pre_train.train src/maxtext/configs/base.yml base_output_directory=${BASE_OUTPUT_DIRECTORY} run_name=${RUN_NAME} num_slices=3 enable_single_controller=True"
```

### Migrated Cluster Toolkit Command

```bash
gcluster job submit --pathways \
  --name "${RUN_NAME}" \
  --cluster "${GKE_CLUSTER}" \
  --project "${PROJECT_ID}" \
  --location "${LOCATION}" \
  --num-slices 3 \
  --image "${DOCKER_IMAGE}" \
  --command "python3 -m maxtext.trainers.pre_train.train src/maxtext/configs/base.yml base_output_directory=${BASE_OUTPUT_DIRECTORY} run_name=${RUN_NAME} num_slices=3 enable_single_controller=True" \
  --pathways-gcs-location "${BASE_OUTPUT_DIRECTORY}" \
  --pathways-elastic-slices 1 \
  --pathways-max-slice-restarts 10 \
  --compute-type ct5lp-hightpu-4t \
  --topology 4x4
```

### Monitoring & Debugging

```bash
# List all active workloads
gcluster job list

# Stream logs across all pods (essential for multi-host workloads with >5 pods)
gcluster job logs "${RUN_NAME}" --main-only=false

# Inspect the underlying JobSet and Pod resources
kubectl get jobset -l gcluster.google.com/workload="${RUN_NAME}"
kubectl get pods -l jobset.sigs.k8s.io/jobset-name="${RUN_NAME}"
```

---

## 3. MaxText Multi-Tier Checkpointing (MTC) & Emergency Checkpointing

Migrating high-speed in-memory checkpointing workloads with colocated Python sidecars (derived from upstream MaxText PR #5187 & PR #4529).

### Input XPK Command

```bash
xpk workload create-pathways \
  --cluster="${CLUSTER_NAME}" \
  --project="${PROJECT_ID}" \
  --zone="${LOCATION}" \
  --workload="${WORKLOAD_NAME}" \
  --tpu-type=v6e-256 \
  --num-slices=1 \
  --docker-image="${MAXTEXT_IMAGE}" \
  --colocated-python-sidecar-image="${COLOCATED_PYTHON_IMAGE}" \
  --ramdisk-directory="${RAMDISK_DIRECTORY}" \
  --mtc-enabled \
  --pathways-gcs-location="${OUTPUT_PATH}" \
  --command="python3 -m maxtext.trainers.pre_train.train src/maxtext/configs/base.yml run_name=${WORKLOAD_NAME} base_output_directory=${OUTPUT_PATH} enable_multi_tier_checkpointing=True local_checkpoint_directory=${RAMDISK_DIRECTORY}"
```

### Migrated Cluster Toolkit Command

```bash
gcluster job submit --pathways \
  --name "${WORKLOAD_NAME}" \
  --cluster "${CLUSTER_NAME}" \
  --project "${PROJECT_ID}" \
  --location "${LOCATION}" \
  --num-slices 1 \
  --image "${MAXTEXT_IMAGE}" \
  --command "python3 -m maxtext.trainers.pre_train.train src/maxtext/configs/base.yml run_name=${WORKLOAD_NAME} base_output_directory=${OUTPUT_PATH} enable_multi_tier_checkpointing=True local_checkpoint_directory=${RAMDISK_DIRECTORY}" \
  --gke-mtc-ramdisk-dir "${RAMDISK_DIRECTORY}" \
  --pathways-gcs-location "${OUTPUT_PATH}" \
  --pathways-colocated-python-sidecar-image "${COLOCATED_PYTHON_IMAGE}" \
  --gke-mtc-enabled \
  --compute-type ct6e-standard-4t \
  --topology 16x16
```

> [!IMPORTANT]
> **Cluster Prerequisite**: Multi-Tier Checkpointing requires the `HighScaleCheckpointing` and `GcsFuseCsiDriver` GKE addons enabled on the target cluster. If not enabled, `gcluster job submit` fails fast with an actionable error before scheduling pods.

---

## 4. MaxText Reinforcement Learning (RL / GRPO) on TPU 7x

Migrating RL workloads requiring custom proxy arguments and low-latency mesh interconnects (derived from upstream MaxText PR #5183).

### Input XPK Script (`run_qwen3_30b_rl.sh`)

```bash
xpk workload create-pathways \
  --cluster="${CLUSTER_NAME}" \
  --project="${PROJECT_ID}" \
  --zone="${ZONE}" \
  --priority=medium \
  --max-restarts=0 \
  --tpu-type=tpu7x-128 \
  --num-slices=1 \
  --docker-image="${DOCKER_IMAGE}" \
  --workload="${WORKLOAD_NAME}" \
  --custom-pathways-proxy-server-args="${XLA_FLAGS}" \
  --command="${MAXTEXT_COMMAND}" \
  --pathways-gcs-location="gs://<YOUR_PATHWAYS_STATE_BUCKET>/tmp"
```

### Migrated Cluster Toolkit (`gcluster`) Submission

```bash
gcluster job submit --pathways \
  --name "${WORKLOAD_NAME}" \
  --cluster "${CLUSTER_NAME}" \
  --project "${PROJECT_ID}" \
  --location "${ZONE}" \
  --num-slices 1 \
  --image "${DOCKER_IMAGE}" \
  --command "${MAXTEXT_COMMAND}" \
  --priority medium \
  --restarts 0 \
  --pathways-gcs-location "gs://<YOUR_PATHWAYS_STATE_BUCKET>/tmp" \
  --pathways-proxy-args "${XLA_FLAGS}" \
  --compute-type tpu7x-standard-4t \
  --topology 4x4x8
```

---

## 5. Multi-Slice TPU v6e Training Workload

Pre-training across multi-slice TPU clusters with deterministically derived topologies.

### Input XPK Command

```bash
xpk workload create \
  --workload="v6e-pretrain" \
  --cluster="ml-cluster" \
  --project="my-project" \
  --zone="us-central1-a" \
  --tpu-type="v6e-256" \
  --num-slices=4 \
  --docker-image="gcr.io/my-project/pretrain:latest" \
  --command="python3 train.py --batch-size=1024"
```

### Migrated Cluster Toolkit Command

```bash
gcluster job submit \
  --name "v6e-pretrain" \
  --cluster "ml-cluster" \
  --project "my-project" \
  --location "us-central1-a" \
  --num-slices 4 \
  --image "gcr.io/my-project/pretrain:latest" \
  --command "python3 train.py --batch-size=1024" \
  --compute-type ct6e-standard-4t \
  --topology 16x16
```

---

## 6. NVIDIA H100 GPU Workload (A3 Mega) & Dynamic Consumption Models

Workload execution and infrastructure deployment with dynamic consumption models (derived from Cluster Toolkit PR #6276, #6279 & #6306).

### 6A. Workload Submission

#### Input XPK Command

```bash
xpk workload create \
  --workload="h100-finetune" \
  --cluster="gpu-cluster" \
  --project="my-project" \
  --zone="us-east5-a" \
  --device-type="h100-mega-80gb-8" \
  --num-nodes=8 \
  --docker-image="gcr.io/my-project/finetune:v1" \
  --command="torchrun --nproc_per_node=8 train.py"
```

#### Migrated Cluster Toolkit Command

```bash
gcluster job submit \
  --name "h100-finetune" \
  --cluster "gpu-cluster" \
  --project "my-project" \
  --location "us-east5-a" \
  --compute-type h100-mega-80gb-8 \
  --num-nodes 8 \
  --image "gcr.io/my-project/finetune:v1" \
  --command "torchrun --nproc_per_node=8 train.py"
```

*(Note: GPU workloads take `--num-nodes` and never `--topology`).*

### 6B. Infrastructure Deployment with Dynamic Consumption

Modern unified blueprints (`examples/gke-a3-megagpu/`, `examples/gke-a3-ultragpu/`, `examples/gke-a4/`) parameterize consumption models via `vars` or node pool module `settings`:

```yaml
vars:
  # Option 1: On-Demand without Reservation (Default)
  spot: false
  reservation_affinity: {consume_reservation_type: "NO_RESERVATION", specific_reservations: []}

  # Option 2: Target a Specific Compute Reservation
  # reservation_affinity: {consume_reservation_type: "SPECIFIC_RESERVATION", specific_reservations: [{name: "my-reservation"}]}

  # Option 3: Spot Consumption
  # spot: true
  # reservation_affinity: {consume_reservation_type: "NO_RESERVATION", specific_reservations: []}

  # Option 4: DWS Flex Start
  # enable_flex_start: true
  # auto_repair: false
  # static_node_count: $(null)
  # reservation_affinity: {consume_reservation_type: "NO_RESERVATION", specific_reservations: []}

  # Option 5: DWS Flex Start + Queued Provisioning
  # kueue_configuration_path: $(ghpc_stage("../dws-sample-workloads/dws-queues.yaml.tftpl"))
  # enable_flex_start: true
  # enable_queued_provisioning: true
  # static_node_count: $(null)
  # reservation_affinity: {consume_reservation_type: "NO_RESERVATION", specific_reservations: []}
```

> [!IMPORTANT]
> `reservation_affinity` is an object variable (`{consume_reservation_type, specific_reservations}`). `gcluster deploy --vars` parses key-value pairs with a CSV reader and cannot carry nested objects. Reservation targeting **must** be edited directly in the deployment YAML file, whereas boolean variables like `spot` and `enable_flex_start` may be passed via `--vars`.
> For Option 5 (DWS Queued Provisioning), submitted jobs target the queue using `--queue dws-queue` (or workload label `kueue.x-k8s.io/queue-name: dws-queue`).
>
> **Blueprint Scoping Note**: The `vars` consumption menu above applies specifically to modern unified GPU blueprints (`examples/gke-a3-megagpu/`, and as of PRs #6276/#6279 `examples/gke-a3-highgpu/`, `examples/gke-a3-ultragpu/`, `examples/gke-a4/`). Other architectures (A4X, G4, H4D, and all TPU families) declare `reservation_affinity` under the accelerator node pool module `settings:`, where specific reservations reference `$(vars.reservation)` (e.g. `specific_reservations: [{name: $(vars.reservation)}]`) and the reservation name is passed via `vars.reservation`.

---

## 7. Managed Lustre Storage Architecture (`examples/gke-managed-lustre.yaml`)

When provisioning a cluster with Google Cloud Managed Service for Lustre:

1. Copy the canonical blueprint:
   ```bash
   cp examples/gke-managed-lustre.yaml my-lustre-cluster.yaml
   ```
2. The blueprint declares:
   - `modules/file-system/managed-lustre` (provisions the Managed Lustre filesystem).
   - `modules/file-system/gke-persistent-volume` (creates the Kubernetes PV and PVC bound to Lustre).
3. The generated PersistentVolumeClaim claim name is **`gke-lustre-instance-pvc`** (derived from the instance ID `gke-lustre-instance` and `-pvc` suffix).
4. Direct workloads to mount the claim either inline via `gcluster job submit`:
   ```bash
   gcluster job submit \
     --name lustre-train \
     --cluster my-lustre-cluster \
     --location us-central1-a \
     --image python:3.10 \
     --mount 'gke-lustre-instance-pvc;/mnt/lustre;rw' \
     --command 'python3 train.py'
   ```
   Or within a raw Kubernetes Pod/JobSet specification:
   ```yaml
   volumes:
   - name: lustre-storage
     persistentVolumeClaim:
       claimName: gke-lustre-instance-pvc
   ```
