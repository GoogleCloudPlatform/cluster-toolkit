# Config Base

The **Config Base** is the first choice you make in a Cluster Config: which orchestrator
runs your cluster. It sets up the parts every cluster needs regardless of hardware, such as
the network and, depending on the base, a GKE cluster or a Slurm controller and login node.
Your compute pools, features, and overlays are added on top of it.

Every Cluster Config has exactly one `config_base`.

---

## 1. Syntax & Quick Examples

In most cases, just name the base:

```yaml
config_base: slurm

vars:
  project_id: my-project      # Replace with your actual Google Cloud Project ID where Compute Engine API is enabled

compute_archetypes:
  - name: cpu-pool
    machine_type: n2-standard-4
```

To change something about the base itself, such as the Slurm controller's machine type, use
the longer form with `type` and `settings`:

```yaml
config_base:
  type: slurm
  settings:
    machine_type: n2-standard-16      # Slurm controller machine type
    enable_login_public_ips: false    # no public IP on the login node

vars:
  project_id: my-project      # Replace with your actual Google Cloud Project ID where Compute Engine API is enabled

compute_archetypes:
  - name: cpu-pool
    machine_type: n2-standard-4
```

To list the available bases:

```bash
./gcluster catalog bases
```

---

## 2. Available Config Bases

| `config_base` | Choose it when you want | What it creates | Your compute pools become |
| :--- | :--- | :--- | :--- |
| `gke` | Containers on Kubernetes | A VPC network with IP ranges for pods and services, two service accounts (one for node pools, one for workloads), and a GKE cluster | GKE node pools |
| `slurm` | A traditional HPC batch scheduler | A VPC network, a Slurm controller, and a Slurm login node, both with public IPs by default. Their machine types depend on your GPU pool (see below) | Slurm partitions |
| `jbvm` | Plain VMs with no scheduler | A VPC network only | Standalone VM instances |

Not every machine type works on every base. For example, `a3-megagpu-8g` supports GKE only (currently). To
see which machine types are available for a base:

```bash
./gcluster catalog archetypes
```

On `slurm`, the controller and login node machine types depend on your GPU pool:

| GPU pool `machine_type` | Controller | Login node |
| :--- | :--- | :--- |
| none (CPU pools only) | `n2-standard-8` | `n2-standard-8` |
| `a3-ultragpu-8g` | `n2d-standard-16` | `n2-standard-8` |
| `a4-highgpu-8g` | `n2d-standard-8` | `n2-standard-8` |
| `a4x-highgpu-4g` | `t2a-standard-2` (Arm) | `t2a-standard-2` (Arm) |

---

## 3. Variables

Each base reads these from your top-level `vars:`:

| Variable | Bases | Default | Notes |
| :--- | :--- | :--- | :--- |
| `project_id` | all | none | **Required.** The Google Cloud project that owns the cluster (replace with your actual project where Compute Engine API is enabled) |
| `deployment_name` | all | `gke-cluster`, `slurm-cluster`, or `jbvm-cluster` | Prefixes resource names, such as the network `<deployment_name>-net-0` |
| `region` | all | `us-central1` | Region of the network's subnet |
| `zone` | all | `us-central1-a` | Set it to a zone where your machine type is available |
| `authorized_cidr` | `gke` | `0.0.0.0/0` | IP range allowed to reach the GKE control plane |

> [!WARNING]
> On `gke`, the default `authorized_cidr` of `0.0.0.0/0` lets any IP address reach the
> control plane (access still requires IAM permissions). Restrict it to your own range,
> for example `authorized_cidr: 203.0.113.5/32`.

---

## 4. Customizing the Base with `settings`

Use `config_base.settings` to customize the infrastructure created by your base (such as the GKE cluster, the Slurm controller and login nodes, or the VPC network).

Settings are declared directly under `settings:` and route automatically:

### Settings Routing Rules

1. **Base Variables**: If a setting matches a base variable (such as `authorized_cidr` on GKE), it updates that variable directly.
2. **Primary Component Priority**: Settings are delivered to the base's primary component if that component declares them as valid inputs:
   - **`slurm`** → **Slurm controller** (e.g., `machine_type: n2d-standard-16`).
   - **`gke`** → **GKE cluster** (e.g., `release_channel: STABLE`).
   - **`jbvm`** → **VPC network** (e.g., `mtu: 1460`).
3. **Auxiliary / Sidecar Routing**: If the primary component does not accept a setting, `gcluster` delivers it to any auxiliary component in the base that accepts it:
   - On `slurm`, settings like `enable_login_public_ips` are not accepted by the controller, so they route automatically to the **login node** (`slurm_login`).
   - Network settings like `subnetworks` or `firewall_rules` route to the **VPC network** (`network`).

### Examples

```yaml
# slurm: customize controller machine type and disable login public IPs
config_base:
  type: slurm
  settings:
    machine_type: n2d-standard-16     # routes to slurm_controller
    enable_login_public_ips: false   # routes to slurm_login
```

> [!NOTE]
> On `a4x-highgpu-4g` clusters, the Slurm controller uses an Arm (`arm64`) OS image. If overriding `machine_type` for the controller, choose an Arm machine type (such as `t2a-*`), as x86 types like `n2-*` cannot boot this image.

```yaml
# gke: set the GKE cluster release channel
config_base:
  type: gke
  settings:
    release_channel: STABLE
```

```yaml
# jbvm: set the network MTU
config_base:
  type: jbvm
  settings:
    mtu: 1460
```

Values you set in `config_base.settings` take priority over any default values set by the base or machine type archetypes.

---

## 5. Rules

`gcluster` checks these before generating anything:

- **Exactly one base.** `config_base` must be `gke`, `slurm`, or `jbvm`. Any other value is
  an error that lists the available bases.
- **If you add `settings`, also write the base name as `type`.** The short form
  `config_base: slurm` is all most configs need. Only when you add `settings` does the base
  name move under `type:`, as in the example in [Section 1](#1-syntax--quick-examples).
  Writing `settings` without `type` is an error.
- **Unknown settings are errors.** A setting that no component of the base accepts is
  rejected. If it looks like a typo (`machin_type`), the error suggests the correct name and
  lists every accepted setting.
- **The machine type must support the base.** Using a machine type on a base it doesn't
  support (for example `a3-megagpu-8g` on `slurm`) is an error that names the supported bases.
