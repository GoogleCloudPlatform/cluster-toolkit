# v2 User Guide

Welcome to the v2 User Guide. This document provides a high-level overview of the v2 architecture and how you can interact with it as a user.

## What is v2?

This is the "v2" evolution of the Google Cloud Cluster Toolkit. It provides a declarative, intent-driven compiler (`gcluster`) that translates high-level infrastructure requests (your "intent") into fully functioning, best-practice Terraform blueprints.

Instead of manually wiring complex Terraform modules, you simply declare:

1. What orchestrator you want (Config Base)
2. What hardware you need (Compute Archetypes)
3. What capabilities your cluster should have (Features)
4. What custom configurations or workloads to apply (Overlays)

The compiler wires the pieces together for you.

## Building Blocks

To make this possible, v2 relies on a modular configuration model. The catalog (in the `v2/` directory) is split into these core building blocks to promote reusability and separation of concerns.

### [Cluster Config (User Intent)](cluster-configs/README.md)
This is the entry point for users. A cluster config is a single YAML file where you declare the `config_base`, `vars`, `compute_archetypes`, `features`, and `overlays` you want to deploy. It represents the end-user's complete request. Ready-to-use examples live in `cluster-configs/`.

### [Config Base (Orchestrator)](config-base/README.md)
The foundational skeleton of the cluster. This block determines the core orchestrator or environment: `gke`, `slurm`, or `jbvm`. It sets up the network and, for `gke` and `slurm`, the control plane (the GKE cluster, or the Slurm controller and login node).

### [Compute Archetypes (Hardware Pools)](compute-archetypes/README.md)
These define the hardware specifications for your compute pools. Archetypes abstract away the complexities of configuring specific machine families (like `a3-megagpu-8g`, `a4x-highgpu-4g`, or `tpu-v6e`). They include rules for how that hardware should be integrated into different Config Bases. Any standard CPU machine type (for example, `n2-standard-4` or `c2-standard-60`) works through the `cpu` archetype. A GPU or TPU machine type that has no archetype (for example, `a2-highgpu-1g`) is rejected instead of being treated as a CPU machine.

### [Features](features/README.md)
Features are composable infrastructure capabilities. If you need auxiliary systems like storage (Filestore, Managed Lustre, Cloud Storage), observability, or job queuing (`kueue-jobset`), you add a feature. Features carry the necessary module wiring to inject these capabilities into your cluster.

### [Overlays](overlays/README.md)
Overlays introduce pre-packaged workloads, software additions, or specialized settings to the cluster. They supply the payloads and parameters needed to run specific tasks or benchmarks, such as `run-nvidia-smi`, `nccl-jobset-test`, or `fio-bench-job`. The overlays in the catalog today target `gke` clusters.

## Consuming v2

As a user, your primary interaction is through the Cluster Config. For example, a basic configuration looks like this:

```yaml
config_base: slurm                 # Required: gke, slurm, or jbvm
vars:
  deployment_name: my-cluster      # Also the name of the deployment directory
  project_id: my-gcp-project       # Required: Replace with your actual Google Cloud Project ID where Compute Engine API is enabled
  region: us-central1
  zone: us-central1-a
compute_archetypes:                # Required: at least one pool
  - name: compute-pool
    machine_type: n2-standard-4
    settings:                      # Optional: per-pool overrides
      node_count_dynamic_max: 4
features:                          # Optional
  - name: homefs
    type: filestore
    settings:
      local_mount: /home
    # attach_to: [compute-pool]    # Optional: limit to these pools (default: the whole cluster)
overlays: []                       # Optional (catalog overlays currently target gke)
```

A few rules to keep in mind:

* `config_base`, `vars.project_id` (always provide your actual Google Cloud Project ID where Compute Engine API is enabled), and at least one entry in `compute_archetypes` are required.
* `region`, `zone`, and `deployment_name` have defaults (`us-central1`, `us-central1-a`, `<base>-cluster`), but you will usually set them: GPU and TPU pools need a zone where you have capacity. On `gke`, `authorized_cidr` defaults to `0.0.0.0/0` (open to all, IAM still enforced); set it to your own range, for example `<your-ip>/32`.
* Unknown or misspelled keys are rejected, so a typo fails the compile instead of silently dropping part of your cluster. This covers top-level keys and keys under `settings:`. A typo in a feature's settings gets a "did you mean" hint; a typo in a pool's settings lists the valid inputs.
* Pool, feature, and overlay names must be unique. If you leave out `type`, it defaults to `name`.
* `controller`, `login`, and `compute` are reserved and cannot be used as pool names.
* A cluster can define at most one accelerator pool (one GPU or TPU pool). Defining multiple pools of the same accelerator archetype is not supported. You can add any number of CPU pools alongside it.
* A feature without `attach_to` applies to the whole cluster: every pool, plus the controller and login node on Slurm. If you set `attach_to` to specific pools on Slurm, shared storage such as `/home` is not mounted on the login node.

### Discovering what's available

`gcluster catalog` lists the building blocks you can use, without needing a Google Cloud project:

```bash
./gcluster catalog                          # overview of every kind
./gcluster catalog archetypes               # values for compute_archetypes[].machine_type
./gcluster catalog features --base slurm    # only features that work with slurm
./gcluster catalog examples                 # ready-made cluster configs to copy
```

### Compiling and deploying

Once you have authored your Cluster Config, you can pass it to the `gcluster` compiler. `gcluster` recognizes a v2 cluster config automatically. If you want to see what the expanded blueprint looks like before provisioning, use `expand` or `create`:

```bash
# Preview the expanded Cluster Toolkit blueprint
./gcluster expand my-cluster.yaml -o out.yaml

# Generate a deployment directory from your intent
./gcluster create my-cluster.yaml
```

> [!NOTE]
> **Building `gcluster`:** Run `make` (or `go build -o gcluster gcluster.go`) in the repository root to compile the `gcluster` binary. Always use the `gcluster` binary in the repository root: it looks for the `v2/` catalog next to itself, so a copy installed elsewhere won't find it.

Once you are satisfied with the configuration, you can provision the infrastructure on Google Cloud or directly provision it. The deployment directory is named after `vars.deployment_name`:

```bash
./gcluster deploy my-cluster

# Tear it down when you are done
./gcluster destroy my-cluster
```
