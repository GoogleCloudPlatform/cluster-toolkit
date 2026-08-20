---
name: xpk_to_clustertoolkit
description: Migrates xpk CLI usages to ClusterToolkit commands. Use when users want to convert xpk commands to ClusterToolkit configurations or blueprints in markdown or shell scripts. Do not use for other CLI tools
---

<!-- pymarkdown:disable heading-style,single-title -->

# Overview

You are an expert in migrating `xpk` CLI configurations and commands to Cluster
Toolkit (`gcluster`). Your goal is to automate the translation of `xpk` commands
found in Markdown files and Bash scripts into their equivalent Cluster Toolkit
representations.

# Core Workflows

When processing target files to identify and migrate `xpk` commands, follow
these strict rules depending on the complexity of the command. Always ensure
that line continuations (`\`) are parsed correctly and that environment
variable interpolations (e.g., `${CLUSTER_NAME?}`) are preserved gracefully.

## 1. 1:1 Mapping Rule (Simple Commands)

For simple `xpk` commands, perform an inline replacement using your reasoning to
map commands and flags natively to `gcluster`:

* `xpk workload list` -> `gcluster job list` (Note: flag `--filter-by-job`
  maps to `--name-contains`)
* `xpk workload delete` -> `gcluster job cancel` (Note: map `--workload` to
  `--name` or `--filter-by-job` to `--name-contains`)
* `xpk cluster delete` -> `gcluster destroy <deployment_name>`
* `xpk config set` -> `gcluster job config set`
* `xpk inspector` -> `gcluster job logs <workload_name>` (or `gcluster job
  inspect`)
* `xpk info` -> `gcluster cluster info`
* `xpk cluster list` -> `gcluster cluster list`
* `xpk cluster describe` -> `gcluster cluster describe`
* `xpk cluster adapt` -> Remove command (Note: add comment `# xpk cluster
  adapt is not required; prerequisites are checked automatically during
  'gcluster job submit'`)
* `xpk storage list` -> `gcluster cluster volume`

## 2. Complex Commands (`workload create`, `workload create-pathways`, `cluster create`, and `cluster create-pathways`)

**MANDATORY RULE ON WORKLOAD TRANSLATION**:
* **Always use `gcluster job submit`**: When converting any `xpk workload create` or `xpk workload create-pathways` commands, you MUST translate them directly to `gcluster job submit` commands.
* **Avoid pure `kubectl` commands**: Do NOT generate raw Kubernetes manifests (e.g. raw `Job`, `JobSet`, or `Pod` YAMLs applied via `kubectl apply`) as replacements for `xpk workload create`, unless specifically migrating custom operators like KubeRay (`RayJob`). Cluster Toolkit's `gcluster job submit` is the canonical CLI mechanism for workload orchestration.

For complex `xpk` commands requiring extensive flag translation, machine type
lookups, or Pathways configurations, you MUST use the deterministic Python
parser.

**CRITICAL DIRECTIVE**: Always start by using the deterministic Python parser.
To convert complex commands, stitch any line continuations together and run:
`python3 skills/xpk_to_clustertoolkit/scripts/parse_xpk_to_gcluster.py "xpk ..."`

The script will output the exact `gcluster` command string or configuration
variables you need. If the script drops or fails to map a specific flag, or
encounters an error, manually map that missing flag or correct the output.

### 2a. `workload create` & `workload create-pathways` (Job Submission)

* **Standard Workload Creation (`xpk workload create`)**: Converts to
  `gcluster job submit` with mapped compute types and topologies.
* **Pathways Workload Creation (`xpk workload create-pathways`)**: Converts to
  `gcluster job submit --pathways` with all Pathways-specific flags mapped:
  * Adds mandatory `--pathways` flag.
  * `--headless` -> `--pathways-headless`
  * `--proxy-server-image` -> `--pathways-proxy-server-image`
  * `--server-image` -> `--pathways-server-image`
  * `--pathways-gcs-location` -> `--pathways-gcs-location`
  * `--custom-pathways-server-args` -> `--pathways-server-args`
  * `--custom-pathways-proxy-server-args` -> `--pathways-proxy-args`
  * `--custom-pathways-worker-args` -> `--pathways-worker-args`
  * `--elastic-slices` -> `--pathways-elastic-slices`
  * `--max-slice-restarts` -> `--pathways-max-slice-restarts`
  * Env variables: `--env` -> `--env`, `--pathways-proxy-env`,
    `--pathways-server-env`, `--pathways-worker-env`.

**CRITICAL TPU CONSTRAINT**: In `gcluster job submit`, passing `--num-nodes` for
TPU jobs raises an error (`--num-nodes cannot be used with TPU jobs`).
`gcluster` auto-calculates TPU node count from the slice topology. The parser
automatically omits `--num-nodes` for TPU compute types (`v4`, `v5p`, `v6e`,
`tpu7x`, etc.).

**Standard Example:**

```bash
# Input:
xpk workload create --workload my-workload --tpu-type tpu7x-4x4x4

# Output:
gcluster job submit --name my-workload --compute-type tpu7x-standard-4t --topology 4x4x4
```

**Pathways Example:**

```bash
# Input:
xpk workload create-pathways --workload my-pw-job --tpu-type tpu7x-4x4x4 --headless --pathways-gcs-location gs://my-bucket/tmp

# Output:
gcluster job submit --pathways --name my-pw-job --pathways-gcs-location gs://my-bucket/tmp --compute-type tpu7x-standard-4t --topology 4x4x4
```

### 2b. Blueprint Generation Rule (`cluster create` & `cluster create-pathways`)

For `xpk cluster create` or `xpk cluster create-pathways`, extract the cluster
configuration into a Cluster Toolkit blueprint YAML file:

1. **Asset Selection (Template)**: Determine hardware from `--device-type` or
   `--tpu-type`. Run `bash skills/xpk_to_clustertoolkit/scripts/fetch_toolkit_examples.sh` to examine the
   templates downloaded to `~/.cache/cluster-toolkit-examples/examples` (e.g.,
   `gke-tpu-v6e`, `gke-a3-mega`, `gke-tpu-7x`). Fall back to
   `skills/xpk_to_clustertoolkit/references/gke-a4.yaml` (GPU) or `skills/xpk_to_clustertoolkit/references/gke-tpu-7x.yaml` (TPU).
2. **Extract and Map Variables**: Run: `python3
   skills/xpk_to_clustertoolkit/scripts/parse_xpk_to_gcluster.py "xpk cluster create..."` or `"xpk cluster
   create-pathways..."` The script outputs the `vars` block for your blueprint.

   **Key `vars` Mapping Example:**

   ```yaml
   blueprint_name: my-cluster
   vars:
     deployment_name: my-cluster          # From --cluster
     project_id: my-project               # From --project
     region: us-central1                  # Extracted from --zone
     zone: us-central1-a                  # From --zone
     machine_type: ct6e-standard-4t       # Mapped from --tpu-type / --device-type
     tpu_topology: 4x4                    # Mapped from --tpu-type / --device-type
     num_slices: 2                        # From --num-slices
     reservation: my-reservation          # From --reservation
     spot: true                           # From --spot
   ```

3. **Pathways Cluster Configuration**: When migrating `xpk cluster
   create-pathways` or `xpk cluster create --enable-pathways`, ensure the
   `vars` block includes:

   ```yaml
   vars:
     enable_pathways_for_tpus: true
     pathways_gce_machine_type: n2-standard-64  # Or value from --pathways-gce-machine-type
   ```

   This automatically provisions the dedicated `cpu-np` CPU node pool and
   configures Kueue with Pathways resource flavors.

4. **Execution Commands**: Replace the `xpk cluster create` command with direct `gcluster deploy`:

   ```bash
   gcluster deploy <blueprint-file.yaml> --vars project_id=<project>,deployment_name=<cluster-name>,zone=<zone>
   ```

   **DIRECTIVE**: Always use `gcluster deploy` directly without the `gcluster create` pre-step. `gcluster deploy` accepts the blueprint YAML and `--vars` arguments directly, streamlining provisioning into a single step.

### Detailed Flag Mapping Reference

| XPK Flag / Subcommand | Cluster Toolkit (`gcluster`) Equivalent | Notes & Scope |
| :--- | :--- | :--- |
| `xpk workload create` | `gcluster job submit` | Job submission |
| `xpk workload create-pathways` | `gcluster job submit --pathways` | Adds `--pathways` flag |
| `--workload` | `--name` | Workload identifier |
| `--tpu-type` / `--device-type` | `--compute-type` + `--topology` | Looked up via `pkg/config/machine_mappings.json` |
| `--num-slices` | `--num-slices` | Number of slices |
| `--num-nodes` (GPU / CPU) | `--num-nodes` | OMITTED for TPU jobs |
| `--docker-image` | `--image` | Container image |
| `--base-docker-image` | `--base-image` | Base image for Crane builds |
| `--script-dir` | `--build-context` | Directory for Crane builds |
| `--command` | `--command` | Container entrypoint |
| `--priority` | `--priority` | Priority class |
| `--max-restarts` | `--restarts` | Max restarts before job failure |
| `--ttl-seconds-after-finished` | `--gke-ttl-after-finished` | JobSet retention duration |
| `--termination-grace-period-seconds` | `--grace-period` | Pod termination grace period |
| `--use-parallel-containers` | `--gke-disable-parallel-containers` | Inverted boolean flag polarity |
| `--scheduler` | `--gke-scheduler` | e.g. `gke.io/topology-aware-auto` |
| `--ramdisk-directory` | `--gke-mtc-ramdisk-dir` | Multi-tier checkpointing ramdisk |
| `--mtc-enabled` | `--gke-mtc-enabled` | Enable MTC |
| `--restart-on-exit-codes` | `--restart-on-exit-codes` | Non-failing exit codes |
| `--service-account` | `--service-account` | Pod K8s service account |
| `--headless` (Pathways) | `--pathways-headless` | Headless mode for Pathways |
| `--proxy-server-image` | `--pathways-proxy-server-image` | Pathways proxy image |
| `--server-image` | `--pathways-server-image` | Pathways RM server image |
| `--pathways-gcs-location` | `--pathways-gcs-location` | GCS bucket for Pathways state |
| `--custom-pathways-server-args` | `--pathways-server-args` | Pass-through RM args |
| `--custom-pathways-proxy-server-args` | `--pathways-proxy-args` | Pass-through proxy args |
| `--custom-pathways-worker-args` | `--pathways-worker-args` | Pass-through worker args |
| `--elastic-slices` | `--pathways-elastic-slices` | Pathways elastic slicing |
| `--max-slice-restarts` | `--pathways-max-slice-restarts` | Pathways slice restarts |
| `xpk cluster create-pathways` | Blueprint `vars`: `enable_pathways_for_tpus: true` | Dedicated CPU pool + Kueue CRDs (Cluster Creation) |
| `--pathways-gce-machine-type` | Blueprint `vars`: `pathways_gce_machine_type` | Machine type for Pathways CPU pool (Cluster Creation blueprint only) |
| `xpk cluster delete` | `gcluster destroy <deployment>` | Destroys cluster infrastructure |
| `xpk inspector` | `gcluster job inspect` or `gcluster job logs` | Workload status & log inspection |
| `xpk storage list` | `gcluster cluster volume` | View attached cluster storage |

## 3. Storage Generation & Blueprint Automation Rule

Unlike `xpk`, which manages storage imperatively via `xpk storage create` and `xpk storage attach` (or requires manual out-of-band `gcloud lustre instances create` and manual `PersistentVolume`/`PersistentVolumeClaim` YAML authoring), Cluster Toolkit manages storage infrastructure declaratively within cluster blueprints during cluster deployment (`gcluster deploy`).

* **Integrated Storage Blueprints (Zero-Touch Storage)**: When migrating guides that use Managed Lustre (e.g., `gke-managed-lustre.yaml`), the blueprint automates VPC network creation, Private Service Access (PSA) peering, Managed Lustre instance provisioning, firewall rules, GKE Lustre CSI driver enablement, and PersistentVolume/PersistentVolumeClaim creation (`lustre-pvc`).
  * **Do NOT keep manual infrastructure commands**: Remove manual `gcloud compute addresses create`, `gcloud services vpc-peerings connect`, `gcloud lustre instances create`, and manual `PersistentVolume` manifests.
  * **Direct Workload Mounting**: RayCluster or JobSet manifests should directly reference the blueprint-provisioned claim: `claimName: lustre-pvc`.
  * **Spot Provisioning**: Spot instances are configured in Cluster Toolkit via blueprint variables (`--vars ...,spot=true`), NOT CLI flags (`--spot`).
* **Job Submission Storage Mounting**: Use `--mount "<src>;<dest>[;<mode>][;options=<options>]"` with `gcluster job submit`.
  * *Flag Translation*: Automatically converts XPK's `--storage <uri>` or `--storage <name>` flags into `--mount "<src>;<dest>;<mode>"`.
  * *GCS Fuse Options*: Note that the `;options=<options>` parameter is strictly validated by `gcluster` and is supported **exclusively for Cloud Storage volumes (`gs://`)**. Appending `options=` to non-GCS volumes will result in a CLI validation error.

## 4. Logging Translation Rule

When migrating scripts, you may encounter complex `gcloud logging read` commands used to fetch logs for Kubernetes containers associated with XPK workloads. Whenever you see a `gcloud logging read` command filtering for a specific workload or pod name (e.g., `resource.labels.pod_name:"${WL_NAME}-"`), replace the entire `gcloud logging read ...` command with the streamlined Cluster Toolkit equivalent:

`gcluster job logs <workload_name> --project <project_id> --cluster <cluster_name> --location <zone>`

## 5. Mandatory Post-Migration Verification & XPK Residual Audit Rule

**CRITICAL QUALITY MANDATE**:
After updating any documentation file, tutorial, guide, codelab, or recipe:
1. **Always run a Case-Insensitive Search for `XPK`**:
   You MUST execute a case-insensitive search (`grep_search` with `CaseInsensitive: true`) for `xpk` across the entirety of every modified file.
2. **Audit All File Sections**:
   Ensure zero residual XPK references remain across all document layers:
   - **Frontmatter & Metadata**: `summary:`, `title:`, keywords, and metadata headers.
   - **Introductions & Overviews**: `## Introduction`, `### What you'll do` lists, and feature summaries.
   - **Environment Variables & Comments**: Check inline bash comments (e.g., `# Created by XPK` -> `# Created by Cluster Toolkit`).
   - **Add-on / Driver Verification**: Ensure instructions state that drivers/add-ons are enabled by Cluster Toolkit blueprints.
   - **Notes & Callouts**: Check aside boxes (e.g., replace `--spot` flag mentions with `spot=true` in `--vars`).
   - **Cleanup & Teardown**: Ensure manual teardowns (`xpk cluster delete`, `gcloud lustre delete`) are replaced with a single `gcluster destroy <deployment_name>`.
3. **Verify Zero Regressions**:
   Only after confirming that 100% of all actionable XPK references have been replaced with valid Cluster Toolkit commands and blueprints can the file update be considered complete.

## 6. Mandatory Link & Blueprint Existence Verification Rule

**CRITICAL LINK & ASSET INTEGRITY MANDATE**:
Before finalizing any recipe or documentation change:
1. **Verify Blueprint File Paths**:
   Whenever referencing a Cluster Toolkit blueprint YAML (e.g., `examples/gke-a4/gke-a4.yaml`, `examples/gke-tpu-v6e/gke-tpu-v6e.yaml`, `examples/gke-a3-highgpu/gke-a3-highgpu.yaml`), always check against the local Cluster Toolkit repository (e.g., in your local cluster-toolkit clone or `~/.cache/cluster-toolkit-examples/examples`) to ensure the exact relative path and nested directory structure exist. Do not assume top-level paths when blueprints reside in subdirectories.
2. **Verify Documentation Links**:
   Ensure all documentation routes (e.g., `[Cluster Toolkit Overview](/cluster-toolkit/docs/overview)`, `[Configure Environment](/cluster-toolkit/docs/setup/configure-environment)`, `[Managed Lustre Guide](/kubernetes-engine/docs/how-to/ct-lustre-tpu)`) correspond to valid, active documentation paths.
3. **Table of Contents (`_book.yaml`) Consistency**:
   When renaming or introducing new replacement guides (e.g. creating `ct-lustre-tpu.md` to supersede `xpk-lustre-tpu.md`), you MUST update the corresponding `_book.yaml` table-of-contents files and all inbound cross-references across sibling documentation files so navigation remains completely unbroken.

## 7. Mandatory Migration Report & Action Items Deliverable Rule

**MANDATORY DELIVERABLE REQUIREMENT**:
Upon completing any migration task or batch of migrations, the agent MUST generate a structured **Migration Report & Action Items** deliverable (as a markdown artifact, e.g. `migration_report.md` or a comprehensive summary).

The report MUST include:
1. **Migration Inventory & Mapping Summary**:
   - Table of modified files, their live documentation paths, and their specific migration actions.
   - Exact mapping applied (e.g., `xpk cluster create` -> `gcluster deploy <blueprint>`, `xpk workload create` -> `gcluster job submit`).
   - Hardware types, topologies, and blueprints selected (e.g., `examples/gke-a4/gke-a4.yaml`, `examples/gke-tpu-v6e/gke-tpu-v6e.yaml`).
2. **Decommissioning & Cleanup Backlog**:
   - Explicitly list all legacy files, orphan includes, or deprecated tabs earmarked for deletion in follow-up cleanup PRs (e.g., `_xpk_intro.md`, `_create_xpk_cluster_optional_network.md`).
3. **User Action Items & Operational Checklist**:
   - Provide concrete, prioritized next steps for the user to verify the deployment in their environment:
     - **Prerequisites & Permissions**: Required IAM roles, project quotas, or API enablement (`gcluster` CLI setup, `container.googleapis.com`, `lustre.googleapis.com`).
     - **Verification / Staging Commands**: Example commands for testing deployment in a sandbox project or viewing local preview renders.
     - **Cleanup Instructions**: Commands to tear down any test clusters (`gcluster destroy <deployment_name>`).

---

# Gaps & Architectural Differences: XPK vs. Cluster Toolkit (`gcluster`)

When migrating workflows from XPK to Cluster Toolkit, be aware of the following feature gaps and design differences:

1. **Imperative vs. Declarative Infrastructure Management**:
   - **XPK**: Imperative model. Clusters and storage resources are created and attached on the fly using CLI flags (`xpk cluster create`, `xpk storage create`, `xpk storage attach`).
   - **Cluster Toolkit**: Declarative IaC model. Infrastructure is defined in YAML blueprints (Terraform modules) and deployed via `gcluster create` and `gcluster deploy`. Storage resources are declared in the blueprint or mounted at submit time.

2. **TPU `--num-nodes` Flag Difference**:
   - **XPK**: Accepts `--num-nodes` alongside `--tpu-type` in `xpk workload create`.
   - **Cluster Toolkit**: `gcluster job submit` explicitly rejects `--num-nodes` for TPU jobs, automatically computing node counts from `--topology`. Passing `--num-nodes` for TPUs cause a validation error.

3. **Image Caching (`xpk cluster cacheimage`)**:
   - **XPK**: Provides `xpk cluster cacheimage` to pre-pull heavy container images onto node pools.
   - **Cluster Toolkit**: Does not have a direct `cacheimage` CLI command. Image loading is handled asynchronously via GKE Image Streaming or standard Artifact Registry authentication during job submission (`--image` or `--base-image` with Crane).

4. **Ray Cluster Support (`xpk cluster create-ray`)**:
   - **XPK**: Imperative model. Uses experimental subcommands (`xpk cluster create-ray`) to provision GKE node pools, install the KubeRay operator, and deploy RayCluster CRs via CLI flags.
   - **Cluster Toolkit**: Declarative model. KubeRay Operator installation and Ray workload submission are separated:
     - **Option A - Native GKE Ray Addon (Recommended)**: Enable GKE's managed Ray operator directly in `gke-cluster` settings (no `kubectl-apply` needed for CRDs/operator):

       ```yaml
       - id: gke-tpu-v6e-cluster
         source: modules/scheduler/gke-cluster
         settings:
           enable_ray_operator: true
       ```

     - **Option B - Self-Managed Operator via `kubectl-apply`**: If `enable_ray_operator` is `false`, install a custom KubeRay operator version:

       ```yaml
       - id: install_kuberay_operator
         source: modules/management/kubectl-apply
         use: [gke-tpu-v6e-cluster]
         settings:
           apply_manifests:
           - source: "https://raw.githubusercontent.com/ray-project/kuberay/v1.3.0/helm-charts/kuberay-operator/crds/kuberay-crds.yaml"
           - source: "https://github.com/ray-project/kuberay/releases/download/v1.3.0/kuberay-operator-manifests.yaml"
       ```

     - **Workload Submission**: Once either option is deployed, submit Ray workloads by applying `RayJob` or `RayCluster` manifests (`kubectl apply -f ray_job.yaml`).

5. **Storage Scope & Lifecycle**:
   - **XPK**: Imperative model. Supports CLI commands to dynamically create, attach, and list zonal storage resources (`xpk storage create`, `xpk storage attach`).
   - **Cluster Toolkit**: Storage infrastructure (Filestore, Lustre, GCS Buckets, Disks) is declared in the blueprint YAML and provisioned during cluster deployment (`gcluster deploy`). `gcluster job submit` CLI flags only handle job-level volume mounts and PVC references for the workload.

6. **Vertex AI Tensorboard Integration (`xpk cluster create --create-vertex-tensorboard`)**:
   - **XPK**: Provisions a Vertex AI Tensorboard instance imperatively during cluster creation via `--create-vertex-tensorboard`, `--tensorboard-region`, and `--tensorboard-name`.
   - **Cluster Toolkit**: Cluster Toolkit does not support Vertex AI Tensorboard resource management. Users who rely on Vertex AI Tensorboard should configure it independently via `gcloud` or Terraform, and reference the Tensorboard log directory in their workload environment variables or volume mounts.

---

# Constraints and Robustness

- **Line Continuations**: Bash line continuations (`\`) must be stitched together during parsing to understand the full command structure.
- **Environment/Bash Variables**: The Python parsing script cannot resolve bash variables (e.g., `${DEVICE_TYPE}`). Manually trace bash variables back to their definitions, evaluate them, pass evaluated values to the parser, and update variable definitions in the script as needed.
- **File Types & Blueprint Placement**:
  - **For Bash Scripts (`.sh`)**: For `xpk cluster create` / `create-pathways`, generate the blueprint YAML and save it in the same directory as the bash script.
  - **For Markdown (`.md`)**: Add generated blueprint YAML as a ` ```yaml ` block immediately *before* the bash execution block, then replace the bash block contents with `gcluster` commands.

---

# Real-World Migration Examples

## Example: MaxText Reinforcement Learning (RL) Workload (`xpk workload create-pathways`)

**Input XPK Command (from MaxText `run_qwen3_30b_rl.sh`):**

```bash
xpk workload create-pathways \
  --cluster=$CLUSTER_NAME \
  --project=$PROJECT_ID \
  --zone=$ZONE \
  --priority=medium \
  --max-restarts=0 \
  --tpu-type=tpu7x-128 \
  --num-slices=1 \
  --docker-image="${DOCKER_IMAGE}" \
  --workload="${WORKLOAD_NAME}" \
  --custom-pathways-proxy-server-args='${XLA_FLAGS}' \
  --command="${MAXTEXT_COMMAND}"
```

**Migrated Cluster Toolkit (`gcluster`) Command:**

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
  --pathways-proxy-args "${XLA_FLAGS}" \
  --compute-type tpu7x-standard-4t \
  --topology 4x4x4
```
