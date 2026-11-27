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

name: xpk-to-clustertoolkit
description: >
  Migrate xpk CLI commands, scripts, and documentation to Cluster Toolkit
  blueprints and gcluster CLI submission commands. Use when migrating
  AI/ML TPU/GPU workloads from xpk to Cluster Toolkit.
compatibility: "Requires python3, gcluster, and Cluster Toolkit blueprints."
metadata:
  author: GoogleCloudPlatform
  status: experimental
  domain: migration
allowed-tools: Bash(python3:*) Bash(./gcluster explain:*) Bash(./gcluster expand:*) Bash(gcluster explain:*) Bash(gcluster expand:*) Bash(kubectl get:*) Bash(kubectl describe:*) Bash(cat:*) Bash(ls:*) Bash(find:*) Bash(git status:*)
---

> [!WARNING]
> This skill is experimental. Always review generated blueprints and command translations before execution, and explicitly request confirmation for any mutating deployments.

> **Scope:** These rules apply to Cluster Toolkit user and operator XPK migration tasks; repository maintainers explicitly developing or refactoring Cluster Toolkit internals under `cmd/`, `pkg/`, `modules/`, or `tools/` may read and edit `.tf` and Go source files normally.

<!-- pymarkdown:disable heading-style,single-title -->

# Overview

You are an expert in migrating `xpk` CLI configurations and commands to Cluster
Toolkit (`gcluster`). Your goal is to automate the translation of `xpk` commands
found in Markdown files and Bash scripts into their equivalent Cluster Toolkit
representations.

## Canonical Documentation & Reference Tier

For deep technical details and comprehensive translation matrices, consult the
authoritative documentation:
1. **Migration Guide**: Read `docs/migration/xpk_to_clustertoolkit.md` (or online at [XPK to Cluster Toolkit Migration Guide](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/migration/xpk_to_clustertoolkit.md)).
2. **Flag Mapping Matrix**: Read [`skills/xpk-to-clustertoolkit/references/flag_mappings.md`](references/flag_mappings.md) for 1:1 flag translation and `--filter-by-status` to `--status` mappings.
3. **Architectural Differences**: Read [`skills/xpk-to-clustertoolkit/references/architectural_differences.md`](references/architectural_differences.md) for IaC lifecycle, KubeRay, and storage comparisons.
4. **End-to-End Examples**: Read [`skills/xpk-to-clustertoolkit/references/examples.md`](references/examples.md) for full worked migration recipes (MaxText RL, multi-slice TPU, NVIDIA GPU, Lustre).

---

# Core Workflows

When processing target files, stitch together line continuations (`\`) and evaluate
unexpanded shell parameters.

## 1. 1:1 Mapping Rule (Simple Commands)

For simple `xpk` commands, perform inline replacements:
* `xpk workload list` -> `gcluster job list` (filters workloads via `--name-contains`).
* `xpk workload delete <name>` -> `gcluster job cancel <name>` (positional workload name; batch deletion via `--filter-by-job` is not supported).
* `xpk cluster delete` -> `gcluster destroy <deployment_directory>` (takes the path to the deployment directory, e.g. `./<deployment_name>`).
* `xpk config set` -> `gcluster job config set` (Cluster Toolkit accepts only `project`, `cluster`, `location`). Setting this preamble in workflow scripts allows subsequent `job submit`, `job list`, and `job logs` commands to omit connection flags.
* `xpk inspector` -> `gcluster job logs <workload_name>` (or `gcluster job inspect <workload_name>`).
* `xpk info` -> `gcluster cluster info`
* `xpk cluster list` -> `gcluster cluster list`
* `xpk cluster describe` -> `gcluster cluster describe`
* `xpk cluster adapt` -> Remove command (`# Prerequisite tools checked via gcluster; cluster CRDs deployed via blueprint`).
* `xpk storage list` -> `gcluster cluster volume`

## 2. Complex Commands (`workload create` & `cluster create`)

**MANDATORY RULE ON WORKLOAD TRANSLATION**:
* **Always use `gcluster job submit`**: Translate all `xpk workload create` or `xpk workload create-pathways` commands to `gcluster job submit`.
* **Avoid pure `kubectl` commands**: Do NOT generate raw Kubernetes manifests (e.g. raw `Job`, `JobSet`, or `Pod` YAMLs applied via `kubectl apply`) as replacements for `xpk workload create`, unless specifically migrating custom operators like KubeRay (`RayJob`).

**Deterministic Parser Invocation**:
Always run the parser with **single quotes** so shell variables (`$CLUSTER`, `$ZONE`) reach the parser unexpanded:
```bash
python3 skills/xpk-to-clustertoolkit/scripts/parse_xpk_to_gcluster.py 'xpk ...'
```
*(Adjust the script path if vendored outside the repository root).* Do NOT execute `gcluster job submit`, `gcluster deploy`, or `gcluster destroy` (even with `--dry-run-out`), and do NOT search the local repository (`grep` / `find`) for hypothetical cluster or workload names from the user's prompt. `skills/xpk-to-clustertoolkit/scripts/parse_xpk_to_gcluster.py` is the authoritative offline translator; output the translated command or `[PROPOSED REMEDIATION PLAN]` directly.

> [!IMPORTANT]
> `parse_xpk_to_gcluster.py` is the authoritative source of truth for flag mappings.
> Give precedence to user experience and correctness: never silently parse or drop unrecognized flags.
> When the parser encounters an unmapped or unrecognized flag, it emits an explicit `# Warning: Unmapped xpk flags were ignored:` note clarifying that the tool is unsure how to translate it automatically.
> When handling these flags: (a) clearly warn the user about any unrecognized flags and explain that the tool is unsure of their translation, (b) record them in the Migration Report's Unmapped Flags table with behavioral impacts, and (c) do NOT leave inline comments naming the legacy XPK flag in executable scripts. If the flag is load-bearing (`--force`, `--dry-run`), ask the user before proceeding.

### 2a. Workload Submission (`gcluster job submit`)

* **Standard TPU Workloads**: Converted to `gcluster job submit --compute-type <type> --topology <AxBxC> --command <cmd>`.
  * **TPU `--num-nodes` Constraint**: `gcluster job submit` rejects `--num-nodes` for TPU jobs (`cmd/job/submit.go:226 (validateTPUFlags)`). The parser automatically omits `--num-nodes` for TPUs.
* **GPU Workloads**: GPU jobs take `--num-nodes` and **never** `--topology`. The parser preserves `--num-nodes` for GPU compute types (e.g., `a3-megagpu-8g`, `h100-mega-80gb-8`).
* **Pathways Workloads (`xpk workload create-pathways`)**:
  * Adds `--pathways` and maps `--headless` -> `--pathways-headless`.
  * `--pathways-gcs-location <gs-uri>` is mandatory in Cluster Toolkit.
  * **Workload Name Length**: Standard names must be <= 28 characters; Pathways names must be <= 22 characters due to Kubernetes 63-byte coordinator label limits (`<name>-pathways-head-0-0.<name>`).
* **Multi-Tier Checkpointing (MTC)**: Maps `--mtc-enabled` -> `--gke-mtc-enabled`, `--ramdisk-directory` -> `--gke-mtc-ramdisk-dir`, and `--colocated-python-sidecar-image` -> `--pathways-colocated-python-sidecar-image`. (Cluster requires `HighScaleCheckpointing` and `GcsFuseCsiDriver` addons).
* **Elastic Slices**: Maps `--elastic-slices` -> `--pathways-elastic-slices` and `--max-slice-restarts` -> `--pathways-max-slice-restarts`.
* **Container Images & Crane Builds**: `--docker-image` designates the direct workload container image (`--image`). `--base-docker-image` paired with `--script-dir` maps to `--base-image` + `--build-context` for Crane builds. Note that xpk rejects combining `--docker-image` with `--base-docker-image` or `--script-dir`; following xpk precedence, `--docker-image` takes precedence and emits `--image` alone without build context.
* **Storage Mounts**: In xpk, `--storage <name>` references a separate Storage object whose mount point, PVC name, and read-only mode live on the CRD rather than the CLI command. The parser emits a failsafe placeholder `--mount "<src>;/mnt/<src>;ro"` (defaulting to read-only `ro`, matching gcluster's default) with a warning to verify the real parameters via `xpk storage list` or `gcluster cluster volume`. Destination paths are automatically deduplicated (`/mnt/data`, `/mnt/data_2`).

**Standard Workload Example:**
```bash
# Input:
xpk workload create --workload my-workload --tpu-type tpu7x-4x4x4 --command "python3 train.py"

# Output:
gcluster job submit --name my-workload --command 'python3 train.py' --compute-type tpu7x-standard-4t --topology 4x4x4
```

**Pathways Workload Example:**
```bash
# Input:
xpk workload create-pathways --workload my-pw-job --tpu-type tpu7x-4x4x4 --headless --pathways-gcs-location gs://my-bucket/tmp --command "python3 train.py"

# Output:
gcluster job submit --pathways --name my-pw-job --command 'python3 train.py' --pathways-gcs-location gs://my-bucket/tmp --pathways-headless --compute-type tpu7x-standard-4t --topology 4x4x4
```

### 2b. Blueprint Generation (`cluster create`)

For `xpk cluster create` and `create-pathways`, generate a declarative Cluster Toolkit blueprint:

> [!IMPORTANT]
> - **Mandatory `ai_assisted: true` on Disk YAMLs**: Every Cluster Toolkit blueprint or deployment YAML file you create or copy on disk **MUST** include `ai_assisted: true` as a top-level key.
> - **Conversational / CLI / Architecture Prompts (No Unsolicited Disk Writes)**: When the user asks a conversational CLI translation or storage/architecture question without asking to create files on disk, **do NOT write `.yaml` blueprints or `migration_report.md` to disk**—present the complete answer (including the matching blueprint templates and storage modules from steps 1 and 6 below) directly in your chat response.

1. **Asset Selection & Blueprint Copying**:
   Never modify files under `examples/` in place — they are shared, CI-tested assets. When answering a CLI translation question, always include the parser's `blueprint_name: <name>`, `vars:`, any `nodepool_module_settings:`, and the `gcluster deploy <blueprint.yaml> --vars ...` command directly in your response (do not replace `--vars` with a separate `-d` deployment file); if the user explicitly asks to write a new or copied Cluster Toolkit blueprint YAML file to disk, name it `<deployment_name>.yaml` and include `ai_assisted: true` at the top level:
   * **TPU v4 / v5e / v5p / v6e**: `examples/gke-tpu-v6e/gke-tpu-v6e.yaml`, `examples/gke-tpu-v4/gke-tpu-v4.yaml`
   * **TPU 7x**: `examples/gke-tpu-7x/gke-tpu-7x.yaml`
   * **NVIDIA GPU (A4 / A3)**: `examples/gke-a4/gke-a4.yaml`, `examples/gke-a3-megagpu/gke-a3-megagpu.yaml`
   * **Managed Lustre**: `examples/gke-managed-lustre.yaml`
   * **Storage (GCS FUSE, Filestore, Persistent Volumes)**: `examples/storage-gke.yaml`
   *(In standalone environments where the repository is not cloned, run `skills/xpk-to-clustertoolkit/scripts/fetch_toolkit_examples.sh` to sync canonical examples to `~/.cache/cluster-toolkit-examples/`, or download directly via `curl https://raw.githubusercontent.com/GoogleCloudPlatform/cluster-toolkit/main/examples/<path>`).*

2. **Additive Merge Semantics**:
   Merging is **additive**: overwrite only the keys the parser emitted and preserve every other key already present in the example's `vars:` block.
   * **Pre-existing Unset Variables**: Canonical examples (like `examples/gke-tpu-v6e/gke-tpu-v6e.yaml`) declare `authorized_cidr` and `reservation` with no default value. Supply them via `--vars` or prompt the user.
   * **Reservation Affinity**: If `--reservation` is omitted in the XPK command, delete the pre-existing `reservation_affinity` block from the accelerator node pool module and the now-unused `reservation` variable from `vars:` (it is the block's only consumer in canonical blueprints; leaving it trips `test_deployment_variable_not_used`).
   * **GPU Blueprints**: Do NOT merge `compute_machine_type` into `examples/gke-a3-megagpu/` or `examples/gke-a4/` — those blueprints default it to the only machine type the node pool supports. Merge only `static_node_count`, `project_id`, `deployment_name`, `zone`, and `region`.

3. **Decision Rule for `vars:` vs `settings:`**:
   The parser emits three distinct configuration buckets:
   * **`vars:`**: Global blueprint variables (e.g. `project_id`, `deployment_name`, `reservation`, `enable_pathways_for_tpus`). In modern unified GPU blueprints (`gke-a3-megagpu`, and as of PRs #6276/#6279 `gke-a3-highgpu`, `gke-a3-ultragpu`, `gke-a4`), consumption models are configured directly in `vars:` (e.g. `vars.spot`, `vars.enable_flex_start`, and `vars.reservation_affinity`).
   * **`cluster_module_settings:`**: Module settings applied directly under `modules/scheduler/gke-cluster` `settings:` (e.g. `authorized_cidr`).
   * **`nodepool_module_settings:`**: Node pool settings applied directly under `modules/compute/gke-node-pool` `settings:` (e.g. `spot`, `enable_flex_start`, `auto_repair`, and for TPU/other blueprints, `reservation_affinity`). When presenting a translated TPU `xpk cluster create` configuration (`--spot`, `--flex`, or `--reservation`), always include the parser's `nodepool_module_settings:` block (e.g. `spot: true` or `enable_flex_start: true`, `auto_repair: false`, `static_node_count: $(null)`, and `reservation_affinity` with `NO_RESERVATION` or `SPECIFIC_RESERVATION`) in your response alongside the `vars:` block and `gcluster deploy` command.
   * *Reference Pattern*: When an example declares a key in `vars:` and references it in settings as `$(vars.X)` (e.g. `enable_pathways_for_tpus: $(vars.enable_pathways_for_tpus)` or `reservation_affinity.specific_reservations[0].name: $(vars.reservation)`), update the value in `vars:` and leave the reference intact. This keeps values `--vars`-settable at deploy time and prevents `test_deployment_variable_not_used` errors.
   * Do NOT add global variables that the example does not declare; explicit module settings block global propagation and trigger `test_deployment_variable_not_used`.

4. **Pathways Configuration**:
   * **When migrating with Pathways (`--enable-pathways`)**: Set `enable_pathways_for_tpus: true` in `vars:`. This automatically provisions the dedicated `cpu-np` node pool (`n4-standard-64`).
   * **When migrating WITHOUT Pathways**: You MUST explicitly set `enable_pathways_for_tpus: false` (emitted automatically by the parser for TPU clusters). Canonical blueprints (e.g. `gke-tpu-v6e.yaml:53 (enable_pathways_for_tpus)`) default it to `true` and will otherwise provision an unrequested CPU node pool. Do not set this variable on GPU blueprints.

5. **Reservation Affinity Targeting**:
   For modern unified GPU blueprints (`gke-a3-megagpu`, and as of PRs #6276/#6279 `gke-a3-highgpu`, `gke-a3-ultragpu`, `gke-a4`), configure the `reservation_affinity` object directly in `vars:`. For TPU and specialized blueprints (`gke-tpu-v6e-pool`, `gke-tpu-7x-pool`, `a4x_pool`), configure `reservation: <name>` in `vars:` and ensure the accelerator node pool module references `- name: $(vars.reservation)` under `settings:`. If the example already defines `reservation_affinity`, edit the existing block rather than adding a second one.

6. **Storage & Persistent Volume Architecture**:
   * A `modules/file-system/*` module (such as `modules/file-system/managed-lustre` or `modules/file-system/cloud-storage-bucket`) provisions storage instances/buckets but creates **no** PVC. Always pair it with `modules/file-system/gke-persistent-volume` (`use: [<cluster>, <fs-module>]`).
   * In `examples/gke-managed-lustre.yaml`, the generated PersistentVolumeClaim claim name is **`gke-lustre-instance-pvc`** (not `lustre-pvc`). Workloads mount this claim directly via `gcluster job submit --mount`.
   * `<src>` resolution: Cloud Storage buckets can be provisioned via `examples/storage-gke.yaml` (`modules/file-system/cloud-storage-bucket` paired with `modules/file-system/gke-persistent-volume`) or mounted via GCS FUSE CSI (`gcluster job submit --mount "gs://bucket/path;/mnt/..."`); PVC provisioned by `gke-persistent-volume` -> bare claim name; Filestore -> `filestore://<instance>/<share>`. `<dest>` was set by `xpk storage create --mount-point` and cannot be derived from the submission command; ask the user rather than guessing.

7. **GHPC Template Expressions & HCL Arithmetic**:
   Template expansions inside `$(...)` evaluate as native HCL expressions. Compound arithmetic is fully supported (e.g. `$(vars.num_slices * pool.node_count_static * pool.tpu_chips_per_node)`).

8. **Deployment Command**:
   ```bash
   gcluster deploy <blueprint_file.yaml> --vars project_id=<project>,deployment_name=<name>,zone=<zone>,region=<region>
   ```
   Blueprint YAML cannot interpolate **any** shell variable — neither `$ZONE` nor `${ZONE%-*}`. Every value the parser emits containing `$` must either be passed at deploy time via `--vars`, or replaced with a literal. Note that `blueprint_name` is a top-level field, not a variable: `--vars` cannot override it, so it must always be a literal lowercase RFC-1035 string (e.g. `my-cluster`). `reservation` must also be passed at deploy time (via `--vars reservation=$RES`) when the source command supplied one.

---

# Safety Guidelines & Confirmation Protocol

* **Safe Actions (Zero Turn Delay)**: Generating blueprints, inspecting configurations (`gcluster expand`), translating CLI commands, parsing recipes, and reading documentation execute freely.
* **Mutating & Billable Actions (Confirmation Strictly Required)**:
  * **Infrastructure Provisioning**: `gcluster deploy` (including re-deployments over existing directories) creates billable Cloud resources and modifies infrastructure.
  * **Workload Execution**: `gcluster job submit` launches billable computing workloads on clusters.
  * **Destructive Teardown**: `gcluster job cancel` and `gcluster destroy` terminate running jobs and delete infrastructure (never include `--auto-approve`, even inside a remediation plan).
  * **Local Destructive Steps**: deleting user files or deployment folders (`rm` / `rm -rf`; prefer `./gcluster create ... -w` over `rm -rf`), or reverting edits via `git checkout` / `git restore` / `git reset` / `git clean`.
  These actions must **NEVER** execute autonomously without presenting a structured remediation plan and obtaining user confirmation.

### Required Remediation Plan Format:

```markdown
[PROPOSED REMEDIATION PLAN]
- **Target Resource**: <Cluster|Workload|Deployment>/<RESOURCE_NAME> (<Accelerator & Node Count, e.g. TPU v6e-256 / 64 nodes>)
- **Root Cause Identified**: <Precise migration or operational intent>
- **Proposed Action**: <Exact command to execute, e.g. gcluster deploy <blueprint.yaml> or gcluster job submit ...>
- **Blast Radius**: <Affected clusters, node pools, running jobs, storage volumes, and billable accelerator impact>
- **Confirmation Required**: Reply 'yes' to proceed.
```

---

# Mandatory Post-Migration Verification & Residual Audit (Workspace File Migrations Only)

> [!IMPORTANT]
> Apply the on-disk verification steps and `migration_report.md` deliverable below **only** when migrating workspace scripts or repositories where the user explicitly asks to create or modify files on disk (for conversational CLI or storage/architecture questions, follow Section 2b above and answer directly in chat without writing files to disk).

1. **Audit for Residual XPK Execution Invocations**:
   Verify that zero residual XPK execution commands remain in migrated workload and deployment scripts. All active runtime entrypoints and submission loops must use `gcluster`. (Comparative documentation and migration guides may retain contextual XPK references for contrast, but operational scripts must be 100% clean).
2. **Verify Asset & Blueprint Existence & `ai_assisted: true`**:
   Verify that all referenced blueprints (e.g. `examples/gke-tpu-v6e/gke-tpu-v6e.yaml`, `examples/gke-managed-lustre.yaml`, `examples/storage-gke.yaml`) exist at the specified relative paths in the workspace, and that every Cluster Toolkit `.yaml` file written to disk includes top-level `ai_assisted: true`.
3. **Generate Migration Report Deliverable (When Migrating On-Disk Scripts/Repos)**:
   Provide a markdown artifact (`migration_report.md`) containing:
   - Migration Inventory & Mapping Summary table.
   - Unmapped Flags table with behavioral impacts.
   - Decommissioning checklist for legacy scripts and tabs.
   - Verification commands and cleanup instructions (`gcluster destroy <deployment_dir>`).
