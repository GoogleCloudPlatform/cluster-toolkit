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

# Flag Mapping Reference: XPK to Cluster Toolkit (`gcluster`)

This reference provides a comprehensive mapping of commands, flags, and options between `xpk` and Cluster Toolkit (`gcluster`).

> [!NOTE]
> `parse_xpk_to_gcluster.py` is the authoritative mapping implementation, validated against xpk v1.16.0. When translating complex commands, always use the script to generate exact command arguments.

---

## 1. Top-Level Command Equivalents

| XPK Subcommand | Cluster Toolkit (`gcluster`) Equivalent | Notes |
| :--- | :--- | :--- |
| `xpk workload create` | `gcluster job submit` | Workload submission. |
| `xpk workload create-pathways` | `gcluster job submit --pathways` | Workload submission with Pathways enabled. |
| `xpk workload delete <name>` | `gcluster job cancel <name>` | Cancels running or queued workload. Positional argument only. |
| `xpk workload list` | `gcluster job list` | Lists workloads. Filter by name using `--name-contains`. |
| `xpk cluster create` | `gcluster deploy <blueprint.yaml>` | Provisions declarative cluster infrastructure. |
| `xpk cluster create-pathways` | `gcluster deploy <blueprint.yaml>` | Blueprint with `enable_pathways_for_tpus: true`. |
| `xpk cluster delete` | `gcluster destroy <deployment_dir>` | Tears down cluster infrastructure. Requires deployment directory path. |
| `xpk cluster list` | `gcluster cluster list` | Lists active clusters in project/location. |
| `xpk cluster describe` | `gcluster cluster describe` | Shows cluster infrastructure details. |
| `xpk info` | `gcluster cluster info` | Displays current cluster context. |
| `xpk inspector` | `gcluster job logs` or `gcluster job inspect` | Inspects workload logs and status. |
| `xpk config set` | `gcluster job config set` | Sets default CLI configuration (`project`, `cluster`, `location` only). |
| `xpk storage list` | `gcluster cluster volume` | Displays attached cluster storage. |
| `xpk cluster adapt` | *(Omit command)* | Prerequisite CLI tools (`gcloud`, `kubectl`, docker auth) are checked during `gcluster job submit`. Cluster-side CRDs (JobSet, Kueue) are deployed by blueprint modules. |

---

## 2. Workload Submission (`job submit`) Flag Mappings

| XPK Flag | Cluster Toolkit (`gcluster`) Flag | Description & Semantics |
| :--- | :--- | :--- |
| `--workload <name>` | `--name <name>` | Workload identifier. Standard workloads maximum 28 characters. Pathways workloads maximum 22 characters due to Kubernetes 63-byte coordinator label limit (`<name>-pathways-head-0-0.<name>`). |
| `--cluster <name>` | `--cluster <name>` | Target GKE cluster name. |
| `--project <id>` | `--project <id>` | Google Cloud project ID. |
| `--zone <zone>` | `--location <zone>` | Google Cloud zone (e.g. `us-central1-a`). |
| `--tpu-type <type>` | `--compute-type` + `--topology` | Hardware identifier mapped via `pkg/config/hardware.go`. |
| `--device-type <type>` | `--compute-type` + `--topology` | Hardware identifier (GPU or TPU). |
| `--num-slices <n>` | `--num-slices <n>` | Number of TPU slices for multi-slice training. |
| `--num-nodes <n>` | `--num-nodes <n>` | Node count for GPU/CPU workloads. **OMITTED** for TPU workloads (calculated from topology). |
| `--docker-image <img>` | `--image <img>` | Container image URI. |
| `--base-docker-image <img>` | `--base-image <img>` or `--image <img>` | Base image for on-cluster Crane builds when paired with `--script-dir`. If `--script-dir` is omitted and no `--docker-image` is passed, maps directly to `--image <img>`. |
| `--script-dir <dir>` | `--build-context <dir>` | Build context directory for Crane image builds. |
| `--command <cmd>` | `--command <cmd>` | Entrypoint command to execute in container. |
| `--priority <tier>` | `--priority <tier>` | Workload priority class (e.g., `high`, `medium`, `low`). |
| `--max-restarts <n>` | `--restarts <n>` | Maximum permitted job restarts before marking failed. |
| `--restart-on-exit-codes <codes>` | `--restart-on-exit-codes <codes>` | Exit codes (1..255) triggering pod restarts. In Pathways, scoped to `pathways-head` where `restartPolicy: Never` is enforced. |
| `--ttl-seconds-after-finished <n>` | `--gke-ttl-after-finished <n>` | Pod/JobSet TTL after completion before cleanup. |
| `--termination-grace-period-seconds <n>` | `--grace-period <n>` | Pod termination grace period in seconds. |
| `--timeout <duration>` | `--timeout <duration>` | Maximum execution duration (e.g. `2h`, `120m`). |
| `--queue <name>` | `--queue <name>` | Target Kueue LocalQueue name. |
| `--gke-namespace <ns>` | `--gke-namespace <ns>` | Kubernetes namespace for job deployment. |
| `--service-account <sa>` | `--service-account <sa>` | Kubernetes Service Account bound to workload. |
| `--scheduler <name>` | `--gke-scheduler <name>` | Custom scheduler (e.g. `gke.io/topology-aware-auto`). |
| `--use-parallel-containers=false` | `--gke-disable-parallel-containers` | Inverted boolean flag. Disables parallel container startup. |
| `--ramdisk-directory <dir>` | `--gke-mtc-ramdisk-dir <dir>` | Directory path for Multi-Tier Checkpointing ramdisk. |
| `--mtc-enabled` | `--gke-mtc-enabled` | Enables Multi-Tier Checkpointing sidecar/annotations. |
| `--wait-for-job-completion` | `--await-job-completion` | Blocks CLI until workload completes or fails. |
| `--enable-debug-logs` | `--verbose` | Emits detailed debug logging during job submission. |
| `--deploy-stacktrace-sidecar` | `--verbose` | Emits detailed debug logging during job submission. |
| `--skip-prereqs` | `--skip-prereqs` | Skips local environment checks (`gcloud`, `kubectl`, docker auth). |
| `--storage <spec>` | `--mount "<src>;<dest>;ro"` | Workload storage mount placeholder. Defaults to safe read-only `;ro` (matching gcluster default); requires verifying PVC, mount point, and mode from XPK Storage CRD before submission. |
| `--mount-options <opts>` | `--mount "...;options=<opts>"` | Supported exclusively for Cloud Storage buckets (`gs://`). |
| `--env <k=v>` | `--env <k=v>` | Environment variables passed to workload container. |
| `--env-file <path>` | *(Not supported)* | Not directly supported by `gcluster job submit`. Expand file contents into repeated `--env <k=v>` flags. |
| `--docker-image-pull-secret <secret>` | `--image-pull-secret <secret>` | Name of the secret used to pull images from private registries. |
| `--cpu-affinity <rule>` | `--cpu-affinity <rule>` | CPU affinity rules (e.g., `numa`). |
| `--gke-custom-templates-path <dir>` | `--gke-custom-templates-path <dir>` | Path to local directory containing custom GKE template overrides. |
| `--gke-nap-provisioning <model>` | `--gke-nap-provisioning <model>` | Compute provisioning model for GKE NAP: `on-demand`, `spot`, `reservation`. |
| `--gke-nap-reservation <name>` | `--gke-nap-reservation <name>` | Google Cloud Reservation name for GKE NAP; required when `--gke-nap-provisioning=reservation`. |
| `--platform <arch>` | `--platform <arch>` | Target platform for image build (e.g. `linux/amd64`, `linux/arm64`). Used with `--base-image`. |

---

## 3. Pathways Workload Flags (`xpk workload create-pathways`)

| XPK Flag | Cluster Toolkit Flag | Notes |
| :--- | :--- | :--- |
| *(Command form)* | `--pathways` | Mandatory flag enabling Pathways workload engine. |
| `--headless` | `--pathways-headless` | Runs Pathways workload in headless daemon mode. |
| `--pathways-gcs-location <uri>` | `--pathways-gcs-location <uri>` | Mandatory GCS bucket path for Pathways coordination state. |
| `--proxy-server-image <img>` | `--pathways-proxy-server-image <img>` | Container image for Pathways proxy server. |
| `--server-image <img>` | `--pathways-server-image <img>` | Container image for Pathways Resource Manager (RM) server. |
| `--pathways-worker-image <img>` | `--pathways-worker-image <img>` | Container image for Pathways worker pods. |
| `--colocated-python-sidecar-image <img>` | `--pathways-colocated-python-sidecar-image <img>` | Remote Python sidecar image running alongside workers for local checkpointing operations. |
| `--pathways-head-np <pool>` | `--pathways-head-np <pool>` | Node pool for Pathways head job (overrides auto-detection of `cpu-np` or `pathways-np`). |
| `--custom-pathways-server-args <args>` | `--pathways-server-args <args>` | Pass-through CLI arguments for server process. |
| `--custom-pathways-proxy-server-args <args>` | `--pathways-proxy-args <args>` | Pass-through CLI arguments for proxy server process. |
| `--custom-pathways-worker-args <args>` | `--pathways-worker-args <args>` | Pass-through CLI arguments for worker processes. |
| `--elastic-slices` | `--pathways-elastic-slices` | Enables elastic slice provisioning. |
| `--max-slice-restarts <n>` | `--pathways-max-slice-restarts <n>` | Permitted restarts per slice before failure. |
| `--pathways-proxy-env <k=v>` | `--pathways-proxy-env <k=v>` | Environment variables injected into proxy pod. |
| `--pathways-server-env <k=v>` | `--pathways-server-env <k=v>` | Environment variables injected into server pod. |
| `--pathways-worker-env <k=v>` | `--pathways-worker-env <k=v>` | Environment variables injected into worker pods. |

---

## 4. Cluster Creation & Slicing Flags (`xpk cluster create`)

| XPK Flag | Cluster Toolkit Blueprint Equivalent | Notes |
| :--- | :--- | :--- |
| `--sub-slicing` | `enable_dynamic_slicing_for_tpus: true` (TPU 7x `vars:`) | Enables GKE Slice Controller for dynamic sub-slicing topologies. Requires GKE >= 1.35.0-gke.274500 and Kueue/JobSet. Non-7x blueprints fall back to `enable_slice_controller: true` on `gke-cluster`. Cannot be combined with `--spot`/`--flex`/`--on-demand` (see note below). |
| `--super-slicing` | `enable_dynamic_slicing_for_tpus: true` (TPU 7x `vars:`) | Enables GKE Slice Controller for super-slicing topologies. Requires GKE >= 1.35.0-gke.274500 and Kueue/JobSet. Non-7x blueprints fall back to `enable_slice_controller: true` on `gke-cluster`. Cannot be combined with `--spot`/`--flex`/`--on-demand` (see note below). |
| `--num-cubes <n>` | `num_slices: <n>` | xpk assigns `num_slices = num_cubes` (`xpk/src/xpk/commands/cluster.py:361`) and rejects the two being different. Only valid with `--super-slicing`. |
| `--pathways-gce-machine-type <type>` | *(Not supported)* | Pathways CPU pool in Cluster Toolkit is hardcoded to `n4-standard-64` (`modules/scheduler/gke-cluster/main.tf:661`). |

> [!WARNING]
> On TPU 7x, dynamic slicing sets `accelerator_topology_mode: PROVISION_ONLY` on the node pool, which Cluster Toolkit only permits when
> `reservation_affinity.consume_reservation_type` is `SPECIFIC_RESERVATION` (`modules/compute/gke-node-pool/main.tf:482`).
> `--spot`, `--flex` and `--on-demand` all force `NO_RESERVATION`, so combining them with a slicing flag produces a blueprint that cannot pass `terraform plan`.
> Use `--reservation` with slicing, or drop the slicing flag.

---

## 5. Two-Branch Compute Consumption & Provisioning Models

GKE supports two distinct compute provisioning models: static node pools and Node Auto-Provisioning (NAP). Consumption options (`--spot`, `--on-demand`, `--reservation`) are routed according to cluster architecture:

| Cluster Architecture | Where XPK Consumption Flags Go | Notes |
| :--- | :--- | :--- |
| **Static Node Pools** (all canonical `examples/gke-*` blueprints) | Blueprint: `modules/compute/gke-node-pool` `settings:` (or `vars:` in unified blueprints) | Declarative IaC deployment. Reservation affinity requires YAML edit (`reservation_affinity` object cannot be passed via `deploy --vars`). |
| **NAP-Enabled Clusters** (GKE Node Auto-Provisioning enabled) | CLI flags: `gcluster job submit --gke-nap-provisioning=...` | Job-level dynamic provisioning. Validated against cluster NAP limits. |

### Consumption Model Equivalents

| XPK Flag | Cluster Toolkit Static Blueprint Equivalent | Cluster Toolkit NAP Job Flag | Notes & Preconditions |
| :--- | :--- | :--- | :--- |
| `--reservation=<name>` | Unified GPU: `vars.reservation_affinity` with `[{name: <name>}]`. TPU / other: node pool `settings:` with `[{name: $(vars.reservation)}]` + `vars.reservation: <name>` | `--gke-nap-provisioning=reservation --gke-nap-reservation=<name>` | Blueprint: object value in `vars:` (unified GPU) or node pool `settings:` (TPU/other). NAP: `--gke-nap-reservation` is mandatory. Exactly one reservation name. |
| `--on-demand` | `reservation_affinity: {consume_reservation_type: NO_RESERVATION, specific_reservations: []}` | `--gke-nap-provisioning=on-demand` | Default on-demand consumption without targeting reservations. |
| `--spot` | `spot: true` + `reservation_affinity` $\implies$ `NO_RESERVATION` | `--gke-nap-provisioning=spot` | Spot consumption. Precondition: requires `NO_RESERVATION`, mutually exclusive with flex/queued. |
| `--flex` | `enable_flex_start: true` + `auto_repair: false` + `static_node_count: $(null)` + `NO_RESERVATION` | *(Configured via blueprint queue)* | DWS Flex Start. All four companion settings required on node pool. Workloads submit to `--queue dws-queue`. |
| *(DWS Queued)* | `enable_queued_provisioning: true` + `kueue_configuration_path` + `NO_RESERVATION` | *(Submit to `--queue dws-queue`)* | DWS Option 3. Workload requires Kueue queue targeting. |

---

## 6. Status Filtering Translation Matrix (`xpk workload list`)

When migrating workload listing commands from `xpk workload list --filter-by-status` to `gcluster job list --status`, apply this value mapping:

| XPK `--filter-by-status` Value | Cluster Toolkit `gcluster job list --status` Value |
| :--- | :--- |
| `RUNNING` | `Running` |
| `QUEUED` | `Pending` |
| `SUCCESSFUL` | `Succeeded` |
| `FAILED` | `Failed` |
| `FINISHED` | *(Run without `--status`, or filter client-side)* |
| `EVERYTHING` | *(Omit `--status` flag entirely)* |

> [!IMPORTANT]
> The `gcluster job list --status` argument is case-sensitive and accepts **only** values in the strict allowlist (`cmd/job/list.go:35-48 (validStatuses)`):
> `Pending`, `Running`, `Succeeded`, `Failed`, `Suspended`.
> Passing `Success` or lowercase names produces a validation error.

---

## 7. Workload Inspection & Log Streaming

- `gcluster job logs <name>` streams container logs.
- `--main-only`: Streams logs exclusively for the main replicated job (`main-job` or `pathways-head`), ignoring background helper and worker pods.
- `--main-only=false`: Essential for multi-host TPU/GPU and Pathways workloads (>5 pods) to aggregate and stream logs across all worker pods.
- Positional naming: `gcluster job cancel <name>` and `gcluster job inspect <name>` use positional arguments, not `--workload`.

---

## 8. Policy for Unmapped Flags

When running `parse_xpk_to_gcluster.py`, check for the line:
```text
# Warning: Unmapped xpk flags were ignored: <flags>
```

For each unmapped flag:
1. **Remove it** from the migrated `gcluster` command.
2. **Record it** in the Migration Report's Unmapped Flags table with an explanation of its behavioral impact.
3. **Do NOT add inline comments** mentioning the unmapped XPK flag in production scripts (this violates the zero-residual-XPK audit).
4. If the flag is behaviorally critical (e.g. `--force`, `--dry-run`, `--docker-name`), halt automated migration and request human guidance.

---

## 9. Logging Translation Rule (`gcloud logging read`)

When migrating scripts, you may encounter complex `gcloud logging read` commands used to fetch Kubernetes container logs for workloads (e.g., `gcloud logging read 'resource.labels.pod_name:"${WL_NAME}-"'`). Replace the entire command with the streamlined Cluster Toolkit equivalent:

```bash
gcluster job logs <workload_name> --project <project_id> --cluster <cluster_name> --location <zone>
```
