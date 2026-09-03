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

# GKE Kueue Cohorts & Accelerator Infrastructure Reference

**Canonical Tutorial**: [https://docs.cloud.google.com/kubernetes-engine/docs/tutorials/kueue-cohort](https://docs.cloud.google.com/kubernetes-engine/docs/tutorials/kueue-cohort)

This reference details Google Kubernetes Engine (GKE) specific Kueue implementations, multi-tenant Cohort borrowing architectures, Dynamic Workload Scheduler (DWS) Flex-start integration, and AI accelerator hardware profiles (NVIDIA GPUs and Google TPUs).

---

## 1. Multi-Tenant Cohort Quota Sharing on GKE

Cohorts allow multiple `ClusterQueue` resources to pool unused accelerator quota dynamically without starving tenant owners.

### Quota Calculations
* **Nominal Quota (`spec.resourceGroups[*].flavors[*].resources[*].nominalQuota`)**:
  Guaranteed capacity allocated to the queue. When workloads within the queue demand $\le$ nominal quota, they are admitted without borrowing.
* **Borrowing Limit (`borrowingLimit`)**:
  * **When Null / Omitted**: Borrowing is **unlimited** within the cohort. The queue can consume all unallocated nominal quota from peer cohort queues.
  * **When Set (`borrowingLimit: <N>`)**: Caps the maximum quota this queue can borrow above its nominal capacity ($\text{Max Capacity} = \text{nominalQuota} + \text{borrowingLimit}$).
* **Lending Limit (`lendingLimit`)**:
  (Introduced in Kueue v0.9+) Restricts how much quota a queue is willing to lend to peers. Protects reserved capacity for anticipated burst traffic.

### Cohort Reclaim & Preemption
When Queue A is borrowing quota from Queue B:
1. Queue B submits a new workload demanding its nominal capacity.
2. If Queue B has `reclaimWithinCohort: LowerPriority` or `Any`, Kueue evicts the borrowing workloads from Queue A with `Evicted: True (Reason: Preempted)`.
3. Queue B immediately reclaims its nominal capacity and unsuspends its workload.

---

## 2. GKE Dynamic Workload Scheduler (DWS Flex-Start)

In GKE AI Hypercomputer clusters, batch jobs often utilize GKE Dynamic Workload Scheduler (DWS) Flex-start to obtain transient GPU/TPU capacity at reduced pricing.

### AdmissionCheck Lifecycle
1. **Quota Reservation**: Workload receives `QuotaReserved: True` in Kueue.
2. **ProvisioningRequest Generation**: Kueue's DWS admission check controller creates a `ProvisioningRequest` CRD (`autoscaling.x-k8s.io/v1beta1`).
3. **Capacity Acquisition**:
   * If Google Cloud capacity is available, the request transitions to `Provisioned: True`. Kueue marks `AdmissionCheck: Ready` and admits the job.
   * If regional capacity is temporarily exhausted, the request transitions to `Failed: True`.
4. **Exponential Backoff**: Kueue places the workload in `AdmissionCheck: Retry` with a backoff delay set in `.status.admissionChecks[*].requeueAfterSeconds`.

---

## 3. GKE Accelerator Hardware Profiles

### NVIDIA GPUs
* **Resource Key**: `nvidia.com/gpu`
* **Node Accelerator Label**: `cloud.google.com/gke-accelerator`
  * A100: `nvidia-tesla-a100`
  * H100: `nvidia-h100-80gb`, `nvidia-h100-mega-80gb`
  * B200: `nvidia-b200`
  * L4: `nvidia-l4`
  * RTX 6000 Ada: `nvidia-rtx-pro-6000`
* **Node Taints**: `nvidia.com/gpu=present:NoSchedule` (Workloads must specify matching toleration).

### Google TPUs
* **Resource Key**: `google.com/tpu`
* **Node Accelerator Label**: `cloud.google.com/gke-tpu-accelerator`
  * TPU v4: `tpu-v4-podslice`
  * TPU v5e: `tpu-v5-lite-podslice`
  * TPU v5p: `tpu-v5p-slice`
  * TPU v6e (Trillium): `tpu-v6e-slice`
* **Topology Architecture**:
  * **3D Torus (v4 & v5p)**: Requires 3-dimensional topologies (e.g. `2x2x1`, `2x2x2`, `4x4x4`, `4x4x8`).
  * **2D Mesh (v5e & v6e)**: Single-plane 2-dimensional topologies (e.g. `1x1`, `2x2`, `2x4`, `4x4`, `4x8`, `8x8`).
* **Node Taints**: `google.com/tpu=present:NoSchedule` (Workloads must specify matching toleration).

---

## 4. Topology-Aware Scheduling (TAS) on GKE

For large-scale distributed training (JobSet, PyTorchJob), network latency across GPU/TPU nodes is critical.

* **TAS Annotations**:
  * `kueue.x-k8s.io/podset-required-topology: "rack"` or `"block"`
  * `kueue.x-k8s.io/podset-preferred-topology: "rack"`
* **Domain Fragmentation**: If 32 GPUs are free across the cluster, but scattered as 8 GPUs per rack, a job requesting 32 GPUs on a single `rack` domain will be rejected as `Inadmissible`.
