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

# Architectural Differences: XPK vs. Cluster Toolkit (`gcluster`)

When migrating workflows from XPK to Cluster Toolkit, understand these core structural differences between the two architectures.

---

## 1. Declarative Infrastructure-as-Code (IaC) vs. Imperative Management

- **XPK (Imperative)**: Infrastructure is provisioned dynamically via imperative CLI commands (`xpk cluster create`, `xpk storage create`, `xpk storage attach`). Cluster configuration is determined at invocation time, and cluster state changes are executed ad hoc.
- **Cluster Toolkit (Declarative)**: Infrastructure is declared in modular YAML blueprints using Terraform and Google Cloud HPC Toolkit (GHPC) modules. Blueprints are deployed reproducibly via `gcluster deploy <blueprint.yaml>`. Re-deployments apply declarative updates safely to the underlying state machine.

---

## 2. TPU Workload Topology & Node Count

- **XPK**: Accepts `--num-nodes` alongside `--tpu-type` in `xpk workload create`.
- **Cluster Toolkit**: `gcluster job submit` explicitly rejects `--num-nodes` for TPU jobs (`cmd/job/submit.go:226 (validateTPUFlags)`). In Cluster Toolkit, TPU node counts are computed deterministically from the slice `--topology` and accelerator machine type (`pkg/config/hardware.go`). Passing `--num-nodes` to a TPU job causes a validation failure.

---

## 3. Container Image Streaming vs. Image Caching

- **XPK**: Provides `xpk cluster cacheimage` to pre-pull heavy container images directly onto node pools before workload execution.
- **Cluster Toolkit**: Does not require an imperative `cacheimage` CLI command. Image loading is handled efficiently and asynchronously through:
  1. **GKE Image Streaming**: Automatically pulls container layer data on-demand during pod initialization without waiting for the full image download.
  2. **Crane Builds**: Staged directly via `--base-image` and `--build-context` during `gcluster job submit`.

---

## 4. Ray Cluster Deployment (`xpk cluster create-ray`)

- **XPK**: Provisions GKE node pools, deploys the KubeRay operator, and configures RayCluster CRDs imperatively through CLI flags.
- **Cluster Toolkit**: Decouples operator management from workload execution:
  - **Option A - Managed GKE Ray Add-on (Recommended)**: Enable the Google-managed Ray operator directly in cluster module settings:
    ```yaml
    - id: gke-tpu-v6e-cluster
      source: modules/scheduler/gke-cluster
      settings:
        enable_ray_operator: true
    ```
  - **Option B - Self-Managed Operator via `kubectl-apply`**: Deploy custom KubeRay Helm manifests or operator releases declaratively via `modules/management/kubectl-apply`.
  - **Workload Submission**: Once deployed, submit Ray jobs using declarative manifests (`kubectl apply -f ray_job.yaml`).

---

## 5. Storage Scope & Persistent Volume Lifecycle

- **XPK**: Manages storage imperatively (`xpk storage create`, `xpk storage attach`).
- **Cluster Toolkit**: Storage infrastructure (Filestore instances, Managed Lustre, Cloud Storage buckets) is declared in the cluster blueprint.
  - **Instance vs. PVC Separation**: Module `modules/file-system/*` provisions storage instances, but does not create Kubernetes volume claims. To make storage mountable by workloads, pair the file system module with `modules/file-system/gke-persistent-volume` (`use: [<cluster>, <fs-module>]`).
  - **Workload Mounts**: `gcluster job submit --mount "<src>;<dest>[;<mode>][;options=<options>]"` attaches storage at job submission. `<src>` resolves as:
    - Cloud Storage: `gs://<bucket>/<path>`
    - Blueprint PersistentVolumeClaim: Bare claim name (e.g. `gke-lustre-instance-pvc`)
    - Direct Filestore: `filestore://<instance>/<share>`

---

## 6. Vertex AI Tensorboard Integration

- **XPK**: Provisions Vertex AI Tensorboard instances imperatively via `--create-vertex-tensorboard`, `--tensorboard-region`, and `--tensorboard-name`.
- **Cluster Toolkit**: Does not manage Vertex AI Tensorboard instances. Configure Tensorboard independently using `gcloud` or Terraform, and point training workloads to the Tensorboard log directory using workload environment variables (`--env`) or shared storage mounts (`--mount`).

---

## 7. Environment Validation vs. Cluster-Side CRDs

- **XPK (`xpk cluster adapt`)**: Imperatively applies missing CRDs and configurations to external clusters.
- **Cluster Toolkit (`--skip-prereqs`)**: Skips **local** environment prerequisite checks (`gcloud`, `kubectl`, docker authentication). It does **not** install cluster-side CRDs. In Cluster Toolkit, all cluster-side dependencies (JobSet controller, Kueue batch scheduling, MPI operator) are declared and installed by the cluster blueprint (e.g., `modules/management/kubectl-apply`).

---

## 8. Pathways Execution Model & Restart Policy Invariants

- **Split Execution Architecture**: Cluster Toolkit partitions Pathways workloads into two replicated jobs within the JobSet:
  1. `pathways-head`: Runs the user's Python training code, JAX client, and coordination runtime under `restartPolicy: Never`.
  2. `worker`: Runs the distributed Pathways runtime workers co-located on the TPU/GPU slices under `restartPolicy: OnFailure`.
- **`podFailurePolicy` vs. `restartPolicy` Contract**: Kubernetes `batch/v1` strictly enforces that `podFailurePolicy` (used by `--restart-on-exit-codes`) is supported ONLY when `restartPolicy` is `Never`. Cluster Toolkit automatically scopes `podFailurePolicy` to the `pathways-head` container where user restart exit codes are handled, preventing JobSet submission rejections on workers.
- **Workload Name Length Constraint**: In Pathways JobSets, the coordinator address label is formatted as `<workload-name>-pathways-head-0-0.<workload-name>`. Because Kubernetes limits label values to 63 bytes, Pathways workload names must not exceed 22 characters.

---

## 9. Multi-Tier Checkpointing (MTC) Architecture

- **XPK**: Imperatively configured ramdisk size and storage buckets at cluster creation time (`--enable-mtc`, `--mtc-ramdisk-size`).
- **Cluster Toolkit**: Decouples cluster prerequisites from per-workload execution:
  - **Cluster Prerequisite**: The GKE cluster must have `HighScaleCheckpointing` and `GcsFuseCsiDriver` addons enabled.
  - **Workload Submission**: Ramdisk paths and sidecars are configured dynamically per-job using `gcluster job submit --gke-mtc-enabled --gke-mtc-ramdisk-dir <dir>` and optional `--pathways-colocated-python-sidecar-image <image>`.

---

## 10. Dynamic Consumption Models in Modern Unified Blueprints

- **Unified GPU Blueprints Scope**: Modern Cluster Toolkit blueprints (`examples/gke-a3-megagpu/`, `examples/gke-a3-highgpu/`, `examples/gke-a3-ultragpu/`, and `examples/gke-a4/`) parameterize consumption options directly in `vars`. Other families (A4X, G4, H4D, and all TPU blueprints) hardcode `reservation_affinity` on the node pool — configure consumption directly in the `modules/compute/gke-node-pool` module `settings:`.
- **Two-Branch Consumption Architecture**:
  - **Static Node Pool Clusters** (all canonical blueprints): Consumption models are declared in the blueprint (either via `vars:` or node pool `settings:`).
  - **Node Auto-Provisioning (NAP) Clusters**: Consumption models can be selected per-workload at job submission via CLI flags: `gcluster job submit --gke-nap-provisioning=<on-demand|spot|reservation> [--gke-nap-reservation=<name>]`.
- **Consumption Model Variables**:
  - **On-Demand (No Reservation)**: `reservation_affinity: {consume_reservation_type: "NO_RESERVATION", specific_reservations: []}`
  - **Specific Reservation**: `reservation_affinity: {consume_reservation_type: "SPECIFIC_RESERVATION", specific_reservations: [{name: "$(vars.reservation)"}]}` with `reservation: "<name>"` declared in `vars:` (or direct literal object in unified GPU blueprints; add `project: "<owner-project>"` for shared cross-project reservations).
  - **Spot Instances**: `spot: true` (requires `reservation_affinity` to be `NO_RESERVATION`).
  - **DWS Flex Start**: `enable_flex_start: true`, `auto_repair: false`, `static_node_count: $(null)`, and `reservation_affinity` to be `NO_RESERVATION`.
  - **DWS Flex Start + Queued Provisioning**: `enable_queued_provisioning: true`, `enable_flex_start: true`, `kueue_configuration_path: $(ghpc_stage("../dws-sample-workloads/dws-queues.yaml.tftpl"))`, `static_node_count: $(null)`, and `reservation_affinity` to be `NO_RESERVATION`. Workloads targeting queued provisioning submit to `--queue dws-queue`.
- **Mutual Exclusivity & Invariants (`modules/compute/gke-node-pool/main.tf`)**:
  Consumption options are strictly mutually exclusive and enforce Terraform module preconditions:

  | Model | Required Settings & Companions | Forbidden Combinations |
  | :--- | :--- | :--- |
  | `SPECIFIC_RESERVATION` | Exactly 1 entry in `specific_reservations: [{name: ...}]` | Cannot be combined with `spot: true`, `enable_flex_start: true`, or `enable_queued_provisioning: true`. |
  | `spot: true` | `reservation_affinity` must be `NO_RESERVATION` | Cannot be combined with `enable_flex_start: true`, `enable_queued_provisioning: true`, or `SPECIFIC_RESERVATION`. |
  | `enable_flex_start: true` | `auto_repair: false`, `static_node_count: $(null)`, `reservation_affinity: NO_RESERVATION` | Cannot be combined with `spot: true` or `SPECIFIC_RESERVATION`. |
  | `enable_queued_provisioning: true` | `reservation_affinity: NO_RESERVATION`, `autoscaling_total_min_nodes: 0` | Cannot be combined with `spot: true` or `placement_policy.type == "COMPACT"` (unless TPU). |

> [!IMPORTANT]
> `reservation_affinity` is an **object** variable (`{consume_reservation_type, specific_reservations}`). `gcluster deploy --vars` parses values as key-value pairs with a CSV reader and cannot carry nested objects. Reservation targeting **must** be configured directly in the blueprint or deployment YAML file, whereas boolean variables like `spot` and `enable_flex_start` can be passed via `--vars`.
