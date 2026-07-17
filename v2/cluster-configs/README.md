# Cluster Config

A **Cluster Config** is the one YAML file you write to describe a cluster. You say which
orchestrator you want, which machines, and optionally which storage and test workloads.
`gcluster` turns this file into a full Cluster Toolkit deployment.

Ready-to-use Cluster Configs live in [`v2/cluster-configs/`](./), one per
supported machine type and orchestrator (for example `a4high-slurm.yaml`, `a3ultra-gke.yaml`,
`cpu-slurm.yaml`). Starting from one of those is the fastest way to begin.

---

## 1. Syntax & Quick Examples

### Smallest valid Cluster Config

```yaml
config_base: slurm            # orchestrator: gke, slurm, or jbvm

vars:
  project_id: my-project      # your Google Cloud project (replace with your actual project where Compute Engine API is enabled)

compute_archetypes:           # at least one pool of machines
  - name: cpu-pool
    machine_type: n2-standard-4
```

Everything else, including `deployment_name`, `region`, `zone`, networking, and the Slurm
controller and login node, comes from defaults.

### A fuller example

```yaml
config_base: gke

vars:
  project_id: my-project      # Replace with your actual Google Cloud Project ID where Compute Engine API is enabled
  deployment_name: my-first-cluster
  region: us-south1
  zone: us-south1-b               # a zone with a3-ultragpu-8g capacity

compute_archetypes:
  - name: pool1
    machine_type: a3-ultragpu-8g

features:
  - name: shared-fs
    type: filestore
    settings:
      local_mount: /shared

overlays:
  - name: gpu-check
    type: run-nvidia-smi          # needs a GPU pool to run on
```

> [!NOTE]
> GPU overlays such as `run-nvidia-smi` and `nccl-jobset-test` need a GPU pool.
> `gcluster` does not check this: on a CPU-only cluster they still compile, but the job
> cannot run.

### Commands

```bash
./gcluster expand my-cluster.yaml -o out.yaml   # compile only; inspect out.yaml
./gcluster create my-cluster.yaml               # write the deployment folder
./gcluster deploy my-first-cluster              # deploy (folder is named after deployment_name)
./gcluster destroy my-first-cluster             # tear everything down
```

To see what you can put in each section:

```bash
./gcluster catalog bases        # values for config_base
./gcluster catalog archetypes   # values for compute_archetypes[].machine_type
./gcluster catalog features     # values for features[].type
./gcluster catalog overlays     # values for overlays[].type
```

Add `--base <gke|slurm|jbvm>` to any of the last three to see only what works on that
orchestrator.

---

## 2. Top-Level Sections

A Cluster Config has exactly five top-level sections:

| Section | Required | What it is | Details |
| :--- | :---: | :--- | :--- |
| `config_base` | **Yes** | The orchestrator: `gke`, `slurm`, or `jbvm` | [Config Base](../config-base/README.md) |
| `vars` | **Yes** | Values for the whole cluster. Must include `project_id` | [Section 3](#3-vars) |
| `compute_archetypes` | **Yes** | Pools of machines. Each pool has a `name` and a `machine_type` | [Compute Archetypes](../compute-archetypes/README.md) |
| `features` | No | Things the cluster has: storage, a dashboard, a job queue | [Features](../features/README.md) |
| `overlays` | No | Ready-made workloads, such as a GPU check or a benchmark | [Overlays](../overlays/README.md) |

Entries in `compute_archetypes`, `features`, and `overlays` share the same basic shape:

```yaml
- name: my-name     # your choice; must be unique within the section
  type: ...         # features and overlays only (compute_archetypes use machine_type)
  settings: {...}   # compute_archetypes and features only; see each section's page
  attach_to: [...]  # optional list of pool names for features only (Not applicable for overlays)
```

If `attach_to` is omitted, the feature applies to every pool.

> [!IMPORTANT]
> **One Accelerator Pool per Cluster:** A cluster can define at most **one** GPU or TPU pool.
>
> - **GPU + GPU** (e.g. `a3-ultragpu-8g` + `a4x-highgpu-4g`) is **not supported**.
> - **GPU + TPU** (e.g. `a3-ultragpu-8g` + `ct6e-standard-4t`) is **not supported**.
> - **Multiple pools of the same accelerator archetype** (e.g. two `a4x-highgpu-4g` pools or two `a3-megagpu-8g` pools) are also **not supported**.
> - You can add any number of **CPU pools** (such as `n2-standard-4` or `c2-standard-60`) alongside your single accelerator pool.
> - To use multiple accelerator types or multiple accelerator pools, deploy each as its own cluster.

---

## 3. `vars`

`vars` holds values that apply to the whole cluster.

| Variable | Required | Default | Notes |
| :--- | :---: | :--- | :--- |
| `project_id` | **Yes** | none | The Google Cloud project that owns everything (replace with your actual project where Compute Engine API is enabled) |
| `deployment_name` | No, but recommended | `gke-cluster`, `slurm-cluster`, or `jbvm-cluster` | Names the deployment folder and prefixes resource names such as the network |
| `region` | No | `us-central1` | |
| `zone` | No | `us-central1-a` | Set it to a zone where your machine type is available |

> [!TIP]
> Always set `deployment_name`. Two clusters in the same project that both keep the default
> name would try to create resources with the same names.

---

## 4. Configuration Precedence & Priority Order

When a setting or variable can be defined in multiple places, the compiler resolves values using a strict priority order. Lower-priority defaults are overridden by higher-priority specifications.

### The Priority Ladder

> **▲ CLI Flags** (`--vars`, `-d`) &nbsp;*(Highest — overrides file vars at runtime)*  
> &nbsp;&nbsp;&nbsp;&nbsp;▲ **Overlays** &nbsp;*(Strictly additive workloads in Phase 1)*  
> &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;▲ **Explicit Section `settings:`** &nbsp;*(Pool- or feature-specific overrides)*  
> &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;▲ **Cluster Config Global `vars:`** &nbsp;*(Cluster-wide user variables)*  
> &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;▲ **Feature Defaults** &nbsp;*(Built-in storage & monitoring defaults)*  
> &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;▲ **Compute Archetype Defaults** &nbsp;*(Built-in machine templates)*  
> &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;▲ **Config Base Defaults** &nbsp;*(Lowest — orchestrator baseline)*

| Priority | Layer | Example | Scope & Behavior |
| :---: | :--- | :--- | :--- |
| **1 (Lowest)** | **Config Base** | `v2/config-base/slurm.yaml` | Default orchestrator infrastructure (controller/login node types, network topology). |
| **2** | **Compute Archetype** | `v2/compute-archetypes/cpu.yaml` | Default machine settings (e.g. default `disk_size_gb: 100`, partition settings). |
| **3** | **Feature** | `v2/features/filestore.yaml` | Default feature settings (e.g. default `local_mount: /shared`, default capacity). |
| **4** | **Cluster Config Global `vars:`** | `vars: disk_size_gb: 150` | Applies cluster-wide across every pool and module that has not explicitly overridden it. |
| **5** | **Explicit Section `settings:`** | `compute_archetypes[].settings` | **Target-specific.** Explicit settings under a specific pool, feature, or `config_base.settings` override global `vars` for that component only. |
| **6** | **Overlays** | `overlays: [{type: run-nvidia-smi}]` | **Strictly additive in Phase 1.** Injects workloads and preset manifests without overwriting user-configured cluster settings. |
| **7 (Highest)** | **CLI Flags (`--vars`, `-d`)** | `--vars disk_size_gb=500` | Runtime overrides passed via command-line flags or deployment files. |

---

### Precedence in Practice

#### Example 1: Resolving a pool setting (`disk_size_gb`)

```yaml
config_base: slurm

vars:
  project_id: my-project      # Replace with your actual Google Cloud Project ID where Compute Engine API is enabled
  disk_size_gb: 150          # (Level 4) Global default for all pools

compute_archetypes:
  - name: pool1
    machine_type: n2-standard-4
    settings:
      disk_size_gb: 300      # (Level 5) Explicit setting for pool1 only

  - name: pool2
    machine_type: n2-standard-4
                             # Unset here: inherits global vars.disk_size_gb (150)
```

- **`pool1` gets 300 GB**: Explicit section `settings:` (Level 5) beats global `vars:` (Level 4).
- **`pool2` gets 150 GB**: Global `vars:` (Level 4) beats the archetype default (Level 2, which is 100 GB).
- If `disk_size_gb` were removed from `vars:`, `pool2` would fall back to the built-in archetype default of 100 GB.
- If you run with `--vars disk_size_gb=500`, the CLI flag overrides the file's `vars:` for `pool2`, but `pool1` still keeps its explicit `settings.disk_size_gb: 300`.

#### Example 2: Command-line overrides (`--vars` and `-d`)

```bash
# my-cluster.yaml sets vars.deployment_name: from-file
./gcluster create my-cluster.yaml                                              # -> from-file
./gcluster create my-cluster.yaml -d deploy.yaml                               # -> from-deploy-file (beats file vars)
./gcluster create my-cluster.yaml -d deploy.yaml --vars deployment_name=cli   # -> cli (beats deploy file and file vars)
```

CLI precedence order: **`--vars`** → **`-d` deployment file** → **`vars:` in your Cluster Config** → **default**.

---

### Overlays are Strictly Additive in Phase 1

In Phase 1, overlays (`overlays:`) can only **add** new resources to a cluster; they never mutate, overwrite, or delete configurations defined in your compute pools, features, or config base.

- When you declare an overlay (such as `run-nvidia-smi`, `nccl-jobset-test`, or `fio-bench-job`), the compiler injects the overlay's prescribed batch job and companion modules.
- The overlay automatically wires into the target compute pools (or targets GPU/TPU accelerator pools by default if unattached).
- It does not alter existing module settings, mount paths, or variables defined in earlier layers.

---

## 5. Rules

`gcluster` checks these before generating anything, and stops with an error if one fails:

- **Only the five sections above are allowed.** A misspelled or unknown top-level key,
  such as `compute_archetype:`, is an error.
- **`config_base`, `vars.project_id`, and at least one `compute_archetypes` entry are
  required.**
- **Every entry needs a `name`**, and names must be unique within `compute_archetypes`,
  within `features`, and within `overlays`.
- **Every pool named in `attach_to` must exist** in `compute_archetypes`.
- **At most one accelerator pool per cluster.** Combining different accelerator archetypes
  (GPU + GPU, or GPU + TPU) in the same cluster is not supported. Defining multiple pools of the
  same accelerator archetype (such as two `a4x-highgpu-4g` or two `a3-megagpu-8g` pools) is
  also not supported. You can add any number of CPU pools alongside the single accelerator
  pool.

Each section has its own additional rules; see its page linked in [Section 2](#2-top-level-sections).
