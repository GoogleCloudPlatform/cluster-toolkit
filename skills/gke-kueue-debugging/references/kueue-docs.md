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

# Official Kueue Concepts & Architecture Reference

**Canonical Documentation**: [https://kueue.sigs.k8s.io/docs/concepts/](https://kueue.sigs.k8s.io/docs/concepts/)

This reference provides deep technical guidance on Kueue core concepts, APIs (`kueue.x-k8s.io/v1beta1` and `v1beta2`), scheduling lifecycles, and admission policies for AI/ML batch workloads.

---

## 1. Two-Stage Admission Lifecycle

Kueue manages batch jobs (Job, JobSet, RayCluster, PyTorchJob, LeaderWorkerSet) by intercepting them at creation time and injecting `.spec.suspend: true`. Jobs are unsuspended only after passing both stages:

1. **Stage 1: Quota Reservation (`QuotaReserved: True`)**
   * The workload matches a `ResourceFlavor` within its assigned `ClusterQueue`.
   * Sufficient nominal or borrowed quota is available.
   * If quota is unavailable, `QuotaReserved: False`, `Reason: Pending`.
   * If flavor requirements (taints, labels) cannot be satisfied, `QuotaReserved: False`, `Reason: Inadmissible`.

2. **Stage 2: AdmissionChecks & Unsuspension (`Admitted: True`)**
   * Once quota is reserved, external admission checks (e.g. GKE DWS Flex-start, TPU Dynamic Slicing) execute asynchronously.
   * When all checks transition to `State: Ready`, Kueue marks `Admitted: True` and sets `.spec.suspend: false` on the parent job, allowing Kubernetes `kube-scheduler` to bind pods to physical nodes.

---

## 2. Core API Resources

### Workload (`workloads.kueue.x-k8s.io`)
* Represents the resource requirements and scheduling state of a job.
* **Tracking Labels**:
  * `kueue.x-k8s.io/job-uid`: UID of the parent job.
* **Key Status Fields**:
  * `.status.conditions`: `QuotaReserved`, `Admitted`, `Evicted`.
  * `.status.admissionChecks`: List of checks with `name`, `state` (`Pending`, `Ready`, `Retry`, `Rejected`), `requeueAfterSeconds`, and `message`.
  * `.status.admission`: Contains `clusterQueue` and `podSetFlavors`.

### LocalQueue (`localqueues.kueue.x-k8s.io`)
* Namespaced proxy pointing to a cluster-scoped `ClusterQueue`.
* **Administrative Controls**:
  * `.spec.stopPolicy`:
    * `None`: Active queue admitting workloads normally.
    * `Hold`: Stops admitting new workloads; currently running workloads continue.
    * `HoldAndDrain`: Stops admitting new workloads and evicts currently admitted workloads.

### ClusterQueue (`clusterqueues.kueue.x-k8s.io`)
* Cluster-scoped quota manager governing resource groups, flavors, cohorts, and preemption.
* **Queueing Strategies**:
  * `BestEffortFIFO`: If the oldest job cannot be admitted, newer jobs with smaller resource requests can jump ahead.
  * `StrictFIFO`: Strict Head-of-Line ordering. If the oldest job cannot fit, all newer jobs are blocked until it admits.
* **Quota Metrics**:
  * `.status.flavorsReservation`: Total quota reserved across admitted workloads and workloads waiting on AdmissionChecks.
  * `.status.flavorsUsage`: Quota consumed by running pods.

### ResourceFlavor (`resourceflavors.kueue.x-k8s.io`)
* Maps abstract resource requests (`nvidia.com/gpu`, `google.com/tpu`, `cpu`, `memory`) to physical node groups via `nodeLabels`, `nodeTaints`, and `tolerations`.

---

## 3. Preemption & Flavor Fungibility

* **`withinClusterQueue`**: Governs whether high-priority workloads can preempt lower-priority workloads within the same `ClusterQueue` (`Never`, `LowerPriority`).
* **`reclaimWithinCohort`**: Governs whether a queue can preempt borrowed quota from cohort peers to satisfy its own nominal quota (`Never`, `LowerPriority`, `Any`).
* **`flavorFungibility`**:
  * `whenCanBorrow`: Governs whether to borrow quota in the current flavor or try the next flavor.
  * `whenCanPreempt`: Set to `TryNextFlavor` to check if subsequent flavors have free quota before evicting running workloads.

---

## 4. Eviction Reasons & Failure Runbooks

| Eviction Reason | Root Cause | Operator Action |
| :--- | :--- | :--- |
| `Preempted` | Higher-priority job reclaimed nominal or cohort quota. | Inspect priority classes and preemption policies in Step 5. |
| `PodsReadyTimeout` | Pods admitted but containers failed to start within timeout. | Check driver daemonsets, image pull errors, or node hardware failures. |
| `AdmissionCheck` | An admission check transitioned to `Retry` or `Rejected`. | Inspect check status and backoff timers (`requeueAfterSeconds`). |
| `Deactivation` | Workload deactivated (`spec.active: false`) following check failure. | Inspect external admission controller logs (DWS or TPU controller). |
| `HoldAndDrain` | Queue paused by administrator with `stopPolicy: HoldAndDrain`. | Review maintenance schedule or patch queue to `stopPolicy: None`. |
