---
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

name: gke-kueue-debugging
description: >
  Debug GKE Kueue batch workload admission, pending jobs, cohort borrowing,
  queues, flavor matching, AdmissionChecks (DWS / TPU slicing), and TAS topology.
  Use when AI/ML jobs remain in Pending, Inadmissible, or Admitted: False state.
compatibility: "Requires kubectl and access to a GKE cluster with Kueue (>=v0.8) installed."
metadata:
  author: GoogleCloudPlatform
  status: experimental
  domain: gke
allowed-tools: Bash(kubectl:*)
allowed_read_only_commands:
  - "kubectl get"
  - "kubectl describe"
  - "kubectl logs"
---

# GKE Kueue Workload & Admission Debugging Playbook

> [!WARNING]
> This skill is **experimental** and under active validation for Cluster Toolkit. Diagnostic queries are read-only, but remediation plans must be reviewed carefully by cluster operators before execution.

Use this playbook when distributed AI/ML training jobs (PyTorchJob, RayCluster, JobSet, Job, LeaderWorkerSet) submitted to GKE remain indefinitely in `Pending`, `Inadmissible`, `Admitted: False`, or experience unexpected evictions.

---

## 1. Diagnostic Triage (Read-Only)

### Step 0: Fast-Path Workload Inspection (SRE First Response)
If investigating a parent Job, JobSet, or RayCluster, locate its generated Workload name first:
```bash
# Method A: Direct lookup via owner UID (checks Job, JobSet, RayCluster, PyTorchJob, LeaderWorkerSet)
JOB_UID=$(kubectl get job,jobset,raycluster,pytorchjob,leaderworkerset <PARENT_JOB_NAME> -n <NAMESPACE> -o jsonpath='{.metadata.uid}' 2>/dev/null)
[ -n "$JOB_UID" ] && kubectl get workloads -n <NAMESPACE> -l kueue.x-k8s.io/job-uid=$JOB_UID

# Method B: Universal filter by parent job name prefix
kubectl get workloads -n <NAMESPACE> | grep <PARENT_JOB_NAME>
```
Always inspect high-level workload events and conditions:
```bash
kubectl describe workload <WORKLOAD_NAME> -n <NAMESPACE>
```
*Look directly at the bottom `Events:` section for `QuotaReserved`, `AdmissionCheckFailed`, `Inadmissible`, `Preempted`, or `WorkloadEvicted`.*

---

### Step 1: Query Workload Conditions, Evictions & LocalQueue Assignment
Inspect workloads in the target namespace. Do NOT query Pods directly -- Kueue suspends Jobs before Pod creation (`.spec.suspend: true`).
```bash
kubectl get workloads.kueue.x-k8s.io -n <NAMESPACE> -o custom-columns='NAME:.metadata.name,QUEUE:.spec.queueName,RESERVED:.status.conditions[?(@.type=="QuotaReserved")].status,RESERVED_REASON:.status.conditions[?(@.type=="QuotaReserved")].reason,MSG:.status.conditions[?(@.type=="QuotaReserved")].message,ADMITTED:.status.conditions[?(@.type=="Admitted")].status,EVICTED:.status.conditions[?(@.type=="Evicted")].status,EVICTED_REASON:.status.conditions[?(@.type=="Evicted")].reason'
```

*Condition Interpretation:*
* `QuotaReserved: False`, `Reason: Pending`: Workload is waiting for quota in the ClusterQueue or Cohort. Proceed to Step 3.
* `QuotaReserved: False`, `Reason: Inadmissible`: Workload requirements cannot be satisfied by any ResourceFlavor (label, taint, or topology mismatch). See `MSG` and proceed to Step 4 and Step 6.
* `QuotaReserved: True`, `Admitted: False`: Quota is reserved, but the workload is gated by AdmissionChecks (e.g., GKE DWS Flex-start, TPU Dynamic Slicing). Inspect checks:
  ```bash
  kubectl get workload <WORKLOAD_NAME> -n <NAMESPACE> -o jsonpath='{range .status.admissionChecks[*]}{"Check: "}{.name}{" | State: "}{.state}{" | RequeueAfter: "}{.requeueAfterSeconds}{"s | Message: "}{.message}{"\n"}{end}'
  ```
* `Evicted: True`: Workload was interrupted after admission. Check `EVICTED_REASON`:
  - `Preempted`: Evicted to reclaim quota or satisfy higher-priority job. Proceed to Step 5.
  - `PodsReadyTimeout`: Worker pods failed to become ready within `waitForPodsReady.timeout` (check pod image pulls / driver daemonsets).
  - `AdmissionCheck`: An admission check transitioned to Retry/Rejected while running.
  - `Deactivation`: Workload deactivated (`spec.active: false`) due to a rejected admission check.
  - `ClusterQueueStopped` / `LocalQueueStopped`: Queue was paused with `stopPolicy: HoldAndDrain`.

---

### Step 2: Verify LocalQueue & Upstream ClusterQueue Binding
Verify that the LocalQueue exists, routes to an active ClusterQueue, and is not administratively stopped:
```bash
kubectl get localqueue <QUEUE_NAME> -n <NAMESPACE> -o custom-columns='NAME:.metadata.name,CLUSTER_QUEUE:.spec.clusterQueue,STOP_POLICY:.spec.stopPolicy,ACTIVE:.status.conditions[?(@.type=="Active")].status,REASON:.status.conditions[?(@.type=="Active")].reason,PENDING:.status.pendingWorkloads,RESERVING:.status.reservingWorkloads,ADMITTED:.status.admittedWorkloads'
```
*Root Cause Check:*
* If `STOP_POLICY` is `Hold` or `HoldAndDrain`, the queue is inactive and will not admit new workloads.
* If `ACTIVE` is False or the resource does not exist, the workload will remain pending indefinitely.
* `RESERVING` indicates workloads that have passed quota checks but are awaiting AdmissionChecks.

---

### Step 3: Audit ClusterQueue Quotas, Cohorts, and Reservation vs. Usage
Query the upstream `ClusterQueue` without dumping full YAML. Extract queue policy, nominal quotas, borrowing limits, reservations, and active usage:
```bash
# 1. Inspect ClusterQueue policy, cohort, and strategy
kubectl get clusterqueue <CLUSTER_QUEUE_NAME> -o jsonpath='{"ClusterQueue: "}{.metadata.name}{"\nCohort: "}{.spec.cohortName}{.spec.cohort}{"\nQueueingStrategy: "}{.spec.queueingStrategy}{"\nStopPolicy: "}{.spec.stopPolicy}{"\nPendingWorkloads: "}{.status.pendingWorkloads}{" | ReservingWorkloads: "}{.status.reservingWorkloads}{" | AdmittedWorkloads: "}{.status.admittedWorkloads}{"\n---\n"}{range .spec.resourceGroups[*].flavors[*]}{"Flavor: "}{.name}{"\n"}{range .resources[*]}{"  Resource: "}{.name}{" | Nominal: "}{.nominalQuota}{" | BorrowingLimit: "}{.borrowingLimit}{"\n"}{end}{end}'

# 2. Check live reservations vs. actual running pod usage
kubectl get clusterqueue <CLUSTER_QUEUE_NAME> -o jsonpath='{"=== FLAVORS RESERVATION (Admitted + Reserving) ===\n"}{range .status.flavorsReservation[*]}{"Flavor: "}{.name}{"\n"}{range .resources[*]}{"  Resource: "}{.name}{" | Reserved: "}{.total}{" | Borrowed: "}{.borrowed}{"\n"}{end}{end}{"=== FLAVORS USAGE (Actively Admitted Pods) ===\n"}{range .status.flavorsUsage[*]}{"Flavor: "}{.name}{"\n"}{range .resources[*]}{"  Resource: "}{.name}{" | Used: "}{.total}{" | Borrowed: "}{.borrowed}{"\n"}{end}{end}'
```
*Root Cause Check:*
* **Head-of-Line (HOL) Blocking**: If `QueueingStrategy` is `StrictFIFO`, an inadmissible older job at the head of the queue blocks all newer jobs across the cluster even if quota is free! Inspect oldest pending workloads for this queue:
  ```bash
  # In target namespace (sorted by creation timestamp to spot head-of-line blockers)
  kubectl get workloads -n <NAMESPACE> --sort-by=.metadata.creationTimestamp -o custom-columns='NAME:.metadata.name,QUEUE:.spec.queueName,RESERVED:.status.conditions[?(@.type=="QuotaReserved")].status,CREATED:.metadata.creationTimestamp' | head -n 10

  # Or across all namespaces sharing this ClusterQueue
  kubectl get workloads -A --sort-by=.metadata.creationTimestamp -o custom-columns='NS:.metadata.namespace,NAME:.metadata.name,QUEUE:.spec.queueName,RESERVED:.status.conditions[?(@.type=="QuotaReserved")].status,CREATED:.metadata.creationTimestamp' | head -n 10
  ```
* **Cohort Saturation**: If `borrowingLimit` is empty/null, borrowing is unlimited up to the cohort's total available quota. If `total >= nominalQuota` and cohort borrowing is exhausted, the queue must wait for active jobs to finish. For multi-tenant quota sharing architectures, refer to the [GKE Kueue Cohort Tutorial](https://docs.cloud.google.com/kubernetes-engine/docs/tutorials/kueue-cohort).
* **Reservation vs. Usage Delta**: If `Reserved` is high but `Used` is 0, quota is tied up by workloads waiting on `AdmissionChecks` (e.g. DWS capacity acquisition).

> [!NOTE]
> **Tenant RBAC Fallback**: If querying `ClusterQueue` returns `Error from server (Forbidden)`, inspect the namespaced admission message directly:
> ```bash
> kubectl get workload <WORKLOAD_NAME> -n <NAMESPACE> -o jsonpath='{.status.conditions[*].message}'
> ```

---

### Step 4: Verify ResourceFlavor Node Selectors, Taints, and Nodepools
When `Reason: Inadmissible`, verify that the node pool hardware labels, taints, and tolerations match:
```bash
# 1. Inspect ResourceFlavor definition, nodeLabels, nodeTaints, and TAS topology
kubectl get resourceflavor <FLAVOR_NAME> -o jsonpath='{"Flavor: "}{.metadata.name}{"\nNodeLabels: "}{.spec.nodeLabels}{"\nNodeTaints: "}{.spec.nodeTaints}{"\nTolerations: "}{.spec.tolerations}{"\nTopologyName: "}{.spec.topologyName}{"\n"}'

# 2. GKE accelerator node inspection (custom columns for GPU/TPU labels, taints, and nodepools)
kubectl get nodes -o custom-columns='NAME:.metadata.name,NODEPOOL:.metadata.labels.cloud\.google\.com/gke-nodepool,GPU:.metadata.labels.cloud\.google\.com/gke-accelerator,TPU:.metadata.labels.cloud\.google\.com/gke-tpu-accelerator,TPU_TOPOLOGY:.metadata.labels.cloud\.google\.com/gke-tpu-topology,TAINTS:.spec.taints' | head -n 25

# 3. Inspect requested accelerator resource types in the workload's PodSets
kubectl get workload <WORKLOAD_NAME> -n <NAMESPACE> -o jsonpath='{range .spec.podSets[*]}{"PodSet: "}{.name}{" | Count: "}{.count}{" | Requests: "}{.template.spec.containers[*].resources.requests}{"\n"}{end}'
```
*Verify that the workload's Pod template (or individual `spec.podSets[*]`):*
1. **NVIDIA GPU Jobs (`nvidia.com/gpu`)**: Has tolerations matching GPU node taints (`nvidia.com/gpu=present:NoSchedule`) and nodeSelector matching `cloud.google.com/gke-accelerator` (e.g., `nvidia-h100-80gb`, `nvidia-h100-mega-80gb`, `nvidia-l4`).
2. **Google TPU Jobs (`google.com/tpu`)**: Has tolerations matching TPU node taints (`google.com/tpu=present:NoSchedule`), nodeSelector matching `cloud.google.com/gke-tpu-accelerator` (e.g. `tpu-v5p-slice`, `tpu-v6e-slice`), and topology matching `cloud.google.com/gke-tpu-topology` (e.g. `2x2x1` for 3D torus, `2x4` for 2D mesh).
3. Has `nodeSelector` / `nodeAffinity` compatible with the flavor's `nodeLabels`.

---

### Step 5: Audit Workload Priority, Flavor Fungibility & Preemption
If a high-priority workload is blocked behind lower-priority workloads, inspect priority classes, fungibility, and preemption policies:
```bash
# 1. Check Workload Priority and WorkloadPriorityClass
kubectl get workload <WORKLOAD_NAME> -n <NAMESPACE> -o jsonpath='{"PriorityClass: "}{.spec.priorityClassName}{" | Priority: "}{.spec.priority}{"\n"}'

# 2. Check ClusterQueue Preemption and Flavor Fungibility
kubectl get clusterqueue <CLUSTER_QUEUE_NAME> -o jsonpath='{"Preemption: "}{.spec.preemption}{"\nFlavorFungibility: "}{.spec.flavorFungibility}{"\n"}'
```
*Preemption Check:*
* If `withinClusterQueue` is `Never`, high-priority workloads cannot preempt lower-priority workloads in the same queue.
* If `reclaimWithinCohort` is `Never`, workloads cannot reclaim borrowed quota from peer ClusterQueues.
* If `FlavorFungibility.whenCanPreempt` is `TryNextFlavor`, Kueue attempts to find available quota in fallback flavors before preempting running workloads.

---

### Step 6: Topology-Aware Scheduling (TAS) & AdmissionCheck Deep-Dive

#### A. Diagnosing Topology-Aware Scheduling (TAS) Failures
When multi-node distributed training jobs specify compact placement:
```bash
# 1. Check if the workload requested a specific topology (TopologyRequest or PodSet annotations)
kubectl get workload <WORKLOAD_NAME> -n <NAMESPACE> -o jsonpath='{range .spec.podSets[*]}{"PodSet: "}{.name}{" | TopologyRequest: "}{.topologyRequest}{" | Annotations: "}{.template.metadata.annotations}{"\n"}{end}'

# 2. Inspect Topology hierarchy and levels
kubectl get topology -o custom-columns='NAME:.metadata.name,LEVELS:.spec.levels'
```
*If nodes are fragmented across racks/blocks, TAS cannot satisfy contiguous block requirements, leaving the job `Inadmissible`.*

#### B. Diagnosing GKE Dynamic Workload Scheduler (DWS Flex) Provisioning
When using DWS Flex-start (`ProvisioningRequest`):
```bash
kubectl get provisioningrequests.autoscaling.x-k8s.io -n <NAMESPACE> -o custom-columns='NAME:.metadata.name,PROVISIONED:.status.conditions[?(@.type=="Provisioned")].status,FAILED:.status.conditions[?(@.type=="Failed")].status,BOOKED:.status.conditions[?(@.type=="BookingExpired")].status'
```
*If `FAILED` is True, DWS rejected the reservation due to regional stockout. The workload will enter `AdmissionCheck: Retry` or `Rejected`.*

---

## 2. Safety Guidelines & Blast Radius Protocol

* **Safe Commands (Zero Turn Delay)**: Read-only commands (`kubectl get`, `kubectl describe`, `kubectl logs`) execute freely during diagnosis.
* **Mutating Commands (Confirmation Strictly Required)**: Deleting workloads, evicting pods, restarting controllers, altering ClusterQueue quota, or changing stop policies must **NEVER** execute without presenting a structured remediation plan and obtaining user confirmation.

### Required Remediation Plan Format:
```markdown
[PROPOSED REMEDIATION PLAN]
- Target Resource: <ClusterQueue|LocalQueue|Workload>/<RESOURCE_NAME>
- Root Cause Identified: <Precise diagnostic finding>
- Proposed Action: <Exact command to be executed>
- Blast Radius: <Affected workloads, namespaces, and cohort tenants>
- Confirmation Required: Reply 'yes' to proceed.
```

### Canonical Non-Destructive Patch Recipes:
* **Unpausing a LocalQueue (`stopPolicy: None`)**:
  ```bash
  kubectl patch localqueue <NAME> -n <NAMESPACE> --type=merge -p '{"spec":{"stopPolicy":"None"}}'
  ```
* **Switching StrictFIFO to BestEffortFIFO (resolving Head-of-Line deadlock)**:
  ```bash
  kubectl patch clusterqueue <NAME> --type=merge -p '{"spec":{"queueingStrategy":"BestEffortFIFO"}}'
  ```
* **Reactivating an Inactive Workload (`spec.active: true`)**:
  ```bash
  kubectl patch workload <NAME> -n <NAMESPACE> --type=merge -p '{"spec":{"active":true}}'
  ```

---

## 3. References & Canonical Documentation

* **Kueue Concepts & Architecture**: [`references/kueue-docs.md`](references/kueue-docs.md) | [kueue.sigs.k8s.io/docs/concepts](https://kueue.sigs.k8s.io/docs/concepts/)
* **GKE Cohorts & DWS Hardware Profiles**: [`references/gke-kueue-docs.md`](references/gke-kueue-docs.md) | [docs.cloud.google.com/kubernetes-engine/docs/tutorials/kueue-cohort](https://docs.cloud.google.com/kubernetes-engine/docs/tutorials/kueue-cohort)
