# Compute Archetypes

A **Compute Archetype** packages a complete, turnkey hardware recipe for a specific Google Cloud machine family. When you declare a compute pool with a `machine_type`, the archetype automatically provisions the instances along with everything that hardware requires—GPU drivers, high-speed multi-NIC/RoCE networking, NVLink domains, local SSD mounting, DCGM telemetry, and topology placement.

Along with `config_base` and `vars`, `compute_archetypes` is required in every Cluster Config: every cluster must define at least one compute pool.

---

## 1. Syntax & Quick Examples

`compute_archetypes:` is a list. Each pool has:

- `name`: a name you choose, such as `gpu-pool`. Use letters, numbers, and hyphens.
- `machine_type` (required): the Google Cloud machine type, such as `a4-highgpu-8g` or
  `n2-standard-8`. See [Section 2](#2-available-machine-types).
- `settings` (optional): changes for this pool only, such as the number of nodes.

### One GPU pool

```yaml
config_base: slurm

vars:
  project_id: my-project      # Replace with your actual Google Cloud Project ID where Compute Engine API is enabled
  region: us-central1
  zone: us-central1-b          # a zone where your machine type is available

compute_archetypes:
  - name: gpu-pool
    machine_type: a4-highgpu-8g
    settings:
      node_count_static: 4     # 4 GPU nodes (the default is 2)
```

### Multiple pools (and multi-archetype rules)

You can declare multiple compute pools under `compute_archetypes` in your Cluster Config:

```yaml
compute_archetypes:
  - name: gpu-pool
    machine_type: a4-highgpu-8g
  - name: cpu-pool
    machine_type: c2-standard-60
    settings:
      node_count_dynamic_max: 20
```

- **Multiple CPU pools:** You can declare multiple CPU pools of the same machine type (for example, two `n2-standard-8` pools with different names and settings).
- **Only one accelerator pool per cluster:** A cluster can define at most **one** GPU or TPU pool. Defining multiple pools of the same accelerator archetype, or combining different accelerator archetypes (such as GPU + GPU like A3 + A4, or GPU + TPU like A4 + TPU v6e), is not supported. You can add any number of CPU pools alongside your single accelerator pool.

### Listing the available archetypes

```bash
./gcluster catalog archetypes              # all archetypes
./gcluster catalog archetypes --base gke   # only archetypes that work on gke
```

---

## 2. Available Machine Types

| `machine_type` | Hardware | `gke` | `slurm` | `jbvm` |
| :--- | :--- | :---: | :---: | :---: |
| `a3-megagpu-8g` | 8 × NVIDIA H100 80GB | ✅ | — | — |
| `a3-ultragpu-8g` | 8 × NVIDIA H200 141GB | ✅ | ✅ | ✅ |
| `a4-highgpu-8g` | 8 × NVIDIA B200 | ✅ | ✅ | ✅ |
| `a4x-highgpu-4g` | 4 × NVIDIA GB200 | ✅ | ✅ | ✅ |
| `ct6e-standard-4t` | TPU v6e (4 chips per node) | ✅ | — | — |
| Any CPU machine type, such as `n2-standard-8` or `c2-standard-60` (except H4D, see below) | CPUs only | ✅ | ✅ | ✅ |

- **CPU pools:** write the real machine type name, such as `n2-standard-8`. The name
  `cpu` that `gcluster catalog archetypes` lists is the archetype name, not a concrete machine type.
- **H4D (`h4d-*`)** CPU machine types aren't supported yet on any base. Using one is an
  error.
- **Other GPU or TPU types**, such as `a2-highgpu-1g` or `g2-standard-8`, aren't supported
  yet. Using one is an error that lists the supported ones.
- **Zones:** GPU and TPU machine types are only available in some zones. Set `zone` (and
  `region`, if the zone isn't in `us-central1`) to a zone that has your machine type.

What a pool becomes and the primary modules created depend on your config base:

| Config base | Each pool becomes | Primary Terraform modules created |
| :--- | :--- | :--- |
| `slurm` | A Slurm partition with the same name (first pool is default) | [`schedmd-slurm-gcp-v6-nodeset`](../../community/modules/compute/schedmd-slurm-gcp-v6-nodeset/README.md) and [`schedmd-slurm-gcp-v6-partition`](../../community/modules/compute/schedmd-slurm-gcp-v6-partition/README.md) |
| `gke` | A GKE node pool attached to the cluster | [`gke-node-pool`](../../modules/compute/gke-node-pool/README.md) |
| `jbvm` | A group of standalone VM instances | [`vm-instance`](../../modules/compute/vm-instance/README.md) |

*(For GPU and TPU machine types, `gcluster` also automatically provisions secondary RoCE networking modules and driver startup scripts.)*

---

## 3. Number of Nodes

Set the number of nodes in the pool's `settings`. The setting name depends on the config base:

| Config base | Setting | Default for GPU and TPU pools | Default for CPU pools |
| :--- | :--- | :--- | :--- |
| `slurm` | `node_count_static`: nodes that are always running | 2 | 0 |
| `slurm` | `node_count_dynamic_max`: extra nodes Slurm starts when jobs need them | 0 | 10 |
| `gke` | `static_node_count` | 1 (`a4x-highgpu-4g` and `ct6e-standard-4t`: 2) | Not set: the node pool scales automatically |
| `jbvm` | `instance_count` | 2 | 1 |

```yaml
compute_archetypes:
  - name: gpu-pool
    machine_type: a4-highgpu-8g
    settings:
      static_node_count: 3     # gke
```

> [!NOTE]
> **TPU v6e:** each node has 4 TPU chips. The default slice is `2x4` (8 chips) on 2 nodes.
> To change the slice, set `tpu_topology` and set `static_node_count` to the number of chips
> divided by 4. For example, `tpu_topology: 4x4` needs `static_node_count: 4`.

---

## 4. Spot VMs, Reservations, and Flex Start

Pools use on-demand VMs by default. To use Spot VMs, a reservation, or Dynamic Workload
Scheduler (DWS) Flex Start instead, add the setting for your config base to the pool's
`settings`:

| Config base | Spot VMs | Reservation | DWS Flex Start |
| :--- | :--- | :--- | :--- |
| `slurm` | `enable_spot_vm: true` | `reservation_name: <NAME>` | `dws_flex: {enabled: true}` |
| `gke` | `spot: true` | `reservation_affinity` block (below) | `enable_flex_start: true` and `auto_repair: false` |
| `jbvm` | `provisioning_model: SPOT` | `provisioning_model: RESERVATION_BOUND` and `reservation_name: <NAME>` | Not available |

Use only one of these per pool.

On GKE, a reservation is a block:

```yaml
compute_archetypes:
  - name: gpu-pool
    machine_type: a4-highgpu-8g
    settings:
      static_node_count: 2
      reservation_affinity:
        consume_reservation_type: SPECIFIC_RESERVATION
        specific_reservations:
          - name: my-reservation
```

---

## 5. Other Settings

Any other `settings` go to the Terraform module that creates the pool's machines. Its
README lists every setting it accepts:

| Config base | Module |
| :--- | :--- |
| `slurm` | [schedmd-slurm-gcp-v6-nodeset](../../community/modules/compute/schedmd-slurm-gcp-v6-nodeset/README.md); partition settings such as `exclusive` and `is_default` go to [schedmd-slurm-gcp-v6-partition](../../community/modules/compute/schedmd-slurm-gcp-v6-partition/README.md) |
| `gke` | [gke-node-pool](../../modules/compute/gke-node-pool/README.md) |
| `jbvm` | [vm-instance](../../modules/compute/vm-instance/README.md) |

Settings apply only to the pool they're written under. Two pools with the same
`machine_type` can have different settings.

```yaml
compute_archetypes:
  - name: big-disk
    machine_type: n2-standard-8
    settings:
      disk_size_gb: 300
  - name: small-disk
    machine_type: n2-standard-8
    settings:
      disk_size_gb: 150
```

---

## 6. Rules

`gcluster` checks these before generating anything:

- **At least one pool.** A Cluster Config without `compute_archetypes` is an error.
- **Pool names must be unique and not empty.** Two pools with the same `name` is an error.
- **Some names are reserved.** You can't name a pool `compute`, `login`, or `controller`.
- **At most one accelerator pool per cluster.** A cluster can define at most **one** GPU or TPU pool.
  Defining multiple pools of the same accelerator archetype or combining different accelerator archetypes
  is not supported. You can add any number of CPU pools alongside the single accelerator pool.
- **The machine type must support your config base.** For example, `a3-megagpu-8g` on
  `slurm` is an error that names the supported bases.
- **The machine type must exist.** When you set `project_id` and `zone`, a CPU machine type
  that doesn't exist in that zone (for example the typo `n2-standrad-8`) is an error.
- **Unknown settings are errors.** A setting the pool's module doesn't accept is rejected,
  and the error lists every accepted setting.
