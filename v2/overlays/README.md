# Overlays

Along with **Cluster Configs**, **Overlays** are one of the two primary places where users interact with the Cluster Toolkit v2.

An **Overlay** is an additive, user-customized solution for your cluster. Overlays are primarily designed for **you to write your own reusable solutions**—such as a team's standard storage and monitoring suite, a custom benchmark environment, or a specialized workload recipe—so you can add it to any cluster simply by listing its name instead of repeating YAML definitions.

In addition to user-authored overlays, the Cluster Toolkit provides a set of **built-in overlays** out of the box (for GPU diagnostics, NCCL performance tests, and storage benchmarks) that can be listed directly in your Cluster Config.

An overlay can bring everything its solution needs: default values for `vars`, extra compute pools, and features. When you list an overlay under `overlays:`, `gcluster` merges all of those additive elements into your deployment. An overlay cannot include other overlays.

In Phase 1, overlays are **strictly additive**: an overlay adds new workloads, features, compute pools, and unset `vars` on top of your Cluster Config, and cannot modify `config_base.settings` or reuse the name of an existing pool or feature in your Cluster Config.

---

## 1. Writing Your Own Overlay (User-Customized Solutions)

Because overlays are intended as user-customized solutions, you can create custom reusable overlays by dropping YAML files into the `v2/overlays/` directory. `gcluster` reads this directory each time it runs, so a new file is usable immediately without rebuilding `gcluster`.

### Overlay File Structure

An overlay file looks like a Cluster Config with two extra fields at the top (`name` and `description`) and no `overlays:` section:

```yaml
# v2/overlays/bench-kit.yaml
name: bench-kit                     # the value users write in `type:`
description: A benchmark pool with its own shared data folder and a dashboard

config_base: slurm                  # optional: limits the overlay to a specific base

vars:                               # default values for the cluster's vars
  zone: us-central1-b

compute_archetypes:                 # pools to add
  - name: bench-pool
    machine_type: n2-standard-8
    settings:
      disk_size_gb: 200

features:                           # features to add
  - name: "{name}-data"
    type: filestore
    attach_to: [bench-pool]
    settings:
      local_mount: /bench-data
  - name: "{name}-dash"
    type: monitoring-dashboard
```

When a user lists it in their Cluster Config:

```yaml
config_base: slurm

vars:
  project_id: my-project      # Replace with your actual Google Cloud Project ID where Compute Engine API is enabled

compute_archetypes:
  - name: cpu
    machine_type: n2-standard-4

overlays:
  - name: team
    type: bench-kit
```

The resulting cluster gets the `cpu` pool plus the overlay's `bench-pool` (200 GB disks), a Filestore share `team-data` mounted at `/bench-data` on `bench-pool`, a dashboard `team-dash`, and `zone: us-central1-b`.

### Overlay Sections & Additive Rules

| Section | Required | What it does | If your Cluster Config already defines the same name |
| :--- | :---: | :--- | :--- |
| `name` | **Yes** | The overlay's type name (written in `type:` under `overlays:`) | — |
| `description` | No | Human-readable summary shown by `./gcluster catalog overlays` | — |
| `config_base` | No | Limits the overlay to a specific base (`config_base: gke`) | Setting `config_base.settings` in an overlay is an error in Phase 1 |
| `vars` | No | Default values for top-level `vars` (or fills `null` placeholders) | Overriding an existing non-null `var` in your Cluster Config is an error in Phase 1 |
| `compute_archetypes` | No | New pools to add, written as in [Compute Archetypes](../compute-archetypes/README.md) | Reusing an existing pool `name` in your Cluster Config is an error |
| `features` | No | New features to add, written as in [Features](../features/README.md) | Reusing an existing feature `name` in your Cluster Config is an error |

An overlay file must have `name` and at least one of `vars`, `compute_archetypes`, or `features`. An `overlays` section inside an overlay file is an error.

### Example: Custom Job Overlay with a Node Pool

```yaml
# v2/overlays/hello-job.yaml
name: hello-job
description: A CPU pool and a job that prints hello on it
config_base: gke

compute_archetypes:
  - name: job-pool
    machine_type: n2-standard-4

features:
  - name: hello-job                 # single feature: named like the overlay
    type: gke-job-template
    attach_to: [job-pool]
    settings:
      image: busybox
      command: [echo, hello]
```

---

## 2. Using Overlays in a Cluster Config

`overlays:` is an optional list in your Cluster Config. Each entry has:

- `name`: A unique name you choose (e.g. `gpu-check`). Don't reuse the name of one of your features (see [Rules](#4-rules--constraints)).
- `type` (required): The overlay to add (e.g. `run-nvidia-smi` or a custom overlay name like `bench-kit`).

### Example: Adding an Overlay to a Cluster Config

```yaml
config_base: gke

vars:
  project_id: my-project      # Replace with your actual Google Cloud Project ID where Compute Engine API is enabled
  region: us-south1
  zone: us-south1-b           # a zone with a3-ultragpu-8g capacity

compute_archetypes:
  - name: h200-pool
    machine_type: a3-ultragpu-8g

overlays:
  - name: gpu-check
    type: run-nvidia-smi
```

### Listing Available Overlays

```bash
./gcluster catalog overlays              # list all overlays (built-in + custom)
./gcluster catalog overlays --base gke   # list overlays that work on gke
```

Run `./gcluster catalog --help` to explore all available catalog commands and options.

---

## 3. Built-in Catalog Overlays

The Cluster Toolkit includes pre-packaged built-in overlays out of the box. Today these provide test jobs for GKE clusters to verify GPUs, networking, and storage post-deployment:

| `type` | Description | Preset Nodes | Config Base |
| :--- | :--- | :---: | :---: |
| [`run-nvidia-smi`](#run-nvidia-smi) | Runs `nvidia-smi` to verify GPU visibility | 1 | `gke` |
| [`nccl-jobset-test`](#nccl-jobset-test) | Runs NCCL `all_reduce_perf` test to measure GPU-to-GPU bandwidth across nodes | 2 | `gke` |
| [`fio-bench-job`](#fio-bench-job) | Runs an `fio` write benchmark on local SSD | 1 | `gke` |

### `run-nvidia-smi`
Checks that NVIDIA drivers and GPUs are functioning on a node.
- **Image:** `nvidia/cuda:12.8.0-runtime-ubuntu24.04`
- **Command:** `nvidia-smi`
- **Nodes:** 1

### `nccl-jobset-test`
Measures GPU-to-GPU inter-node communication bandwidth.
- **Image:** `us-docker.pkg.dev/gce-ai-infra/gpudirect-gib/nccl-plugin-gib-diagnostic:v1.1.2`
- **Command:** runs `nvidia-smi -L`, then `all_reduce_perf -b 8M -e 8G -f 2 -g 1`
- **Nodes:** 2

### `fio-bench-job`
Measures local SSD write throughput with `fio`.
- **Image:** `ubuntu:22.04`
- **Storage:** Requests a 1000 GB local SSD volume mounted at `/scratch-data`.
- **Nodes:** 1

---

## 4. Rules & Constraints

`gcluster` checks these before generating infrastructure:

- **Names must be unique.** Two overlays with the same `name` is an error (`duplicate overlay name "chk" found`).
- **The type must exist.** An unknown `type` lists all available custom and built-in overlays.
- **Config base compatibility.** Using an overlay on an unsupported base (e.g., `run-nvidia-smi` on `slurm`) fails compilation with supported bases listed.
- **GPU overlays need a GPU pool.** `run-nvidia-smi` and `nccl-jobset-test` compile on a CPU-only cluster without a warning, but the job can't run.
