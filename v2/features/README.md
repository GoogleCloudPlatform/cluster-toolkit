# Features

A **Feature** packages a modular infrastructure capability for your cluster—such as a shared file system, an object storage mount, a metrics dashboard, a batch job queue, or custom node startup scripts. When declared under `features:`, `gcluster` provisions the backing Google Cloud resources and automatically wires them to your compute pools.

Features are optional. You can declare as many features as needed, or omit them entirely.

---

## 1. Syntax & Quick Examples

Features are declared as a list under the top-level `features:` key. Each feature requires a user-defined `name:` (for instance identification) and a catalog `type:`.

### Basic Storage Feature

```yaml
features:
  - name: homefs
    type: filestore
    settings:
      filestore_tier: BASIC_SSD
      size_gb: 2560
      local_mount: /home
```

### Scoped Feature Attachment (`attach_to`)
By default, a feature attaches to **all** compute pools in the cluster. To restrict a feature to specific pools, or target Slurm control plane roles, use `attach_to`:

```yaml
features:
  - name: scratch
    type: managed-lustre
    attach_to: [b200-pool]         # attaches only to pool 'b200-pool'
    settings:
      size_gib: 36000
      local_mount: /scratch

  - name: node-setup
    type: startup-script
    attach_to: [controller, login]  # targets Slurm controller & login node
    settings:
      runners:
        - type: shell
          destination: setup-admin.sh
          content: |
            #!/bin/bash
            echo "Configuring admin environment..."
```

### CLI Discovery
Inspect all available features across all orchestrators:

```bash
./gcluster catalog features
```

Filter features supported by a specific orchestrator:

```bash
./gcluster catalog features --base gke
```

---

## 2. Available Features & Catalog Reference

The catalog provides 11 built-in features across file storage, batch orchestration, observability, and bootstrapping:

| `type` | Description | Primary Module | `gke` | `slurm` | `jbvm` |
| :--- | :--- | :--- | :---: | :---: | :---: |
| [`filestore`](#filestore) | Managed NFS file share with optional Private Service Access | [filestore](../../modules/file-system/filestore/README.md) | ✅ | ✅ | ✅ |
| [`managed-lustre`](#managed-lustre) | High-throughput parallel file system for AI/HPC workloads | [managed-lustre](../../modules/file-system/managed-lustre/README.md) | ✅ | ✅ | ✅ |
| [`netapp-volume`](#netapp-volume) | Google Cloud NetApp storage pool and volume | [netapp-volume](../../modules/file-system/netapp-volume/README.md) | — | ✅ | ✅ |
| [`cloud-storage`](#cloud-storage) | Cloud Storage bucket mounted via Cloud Storage FUSE | [cloud-storage-bucket](../../modules/file-system/cloud-storage-bucket/README.md) | ✅ | ✅ | ✅ |
| [`aiml-gcsfuse`](#aiml-gcsfuse) | GCS bucket with four tuned mount profiles (data, checkpoints, serving, general) | [cloud-storage-bucket](../../modules/file-system/cloud-storage-bucket/README.md) | ✅ | ✅ | ✅ |
| [`pre-existing-network-storage`](#pre-existing-network-storage) | Mounts existing external NFS, Filestore, Lustre, or GCS storage | [pre-existing-network-storage](../../modules/file-system/pre-existing-network-storage/README.md) | ✅ | ✅ | ✅ |
| [`gke-hyperdisk`](#gke-hyperdisk) | Dynamic Hyperdisk StorageClass and PVC for GKE pods | [gke-storage](../../modules/file-system/gke-storage/README.md) | ✅ | — | — |
| [`kueue-jobset`](#kueue-jobset) | Installs Kueue queueing, JobSet batch scheduling, and custom K8s manifests | [kubectl-apply](../../modules/management/kubectl-apply/README.md) | ✅ | — | — |
| [`gke-job-template`](#gke-job-template) | Generates Kubernetes batch Job/JobSet submission manifests | [gke-job-template](../../modules/compute/gke-job-template/README.md) | ✅ | — | — |
| [`monitoring-dashboard`](#monitoring-dashboard) | Cloud Monitoring dashboard with HPC, CPU, memory, and network charts | [dashboard](../../modules/monitoring/dashboard/README.md) | ✅ | ✅ | ✅ |
| [`startup-script`](#startup-script) | Custom startup runners (shell scripts, Ansible playbooks, GCS objects) | [startup-script](../../modules/scripts/startup-script/README.md) | — | ✅ | ✅ |

---

## 3. Feature Details & Resource Realization

### `filestore`
Provisions a Google Cloud Filestore instance. Ideal for shared code, home directories, and moderate-throughput shared storage.

- **Created Modules**:
  - `modules/file-system/filestore`: The Filestore instance (`{name}`).
  - `modules/network/private-service-access`: Conditionally created if `connect_mode == 'PRIVATE_SERVICE_ACCESS'`.
  - `modules/file-system/gke-persistent-volume`: (GKE only) Creates Kubernetes PersistentVolume and PersistentVolumeClaim (`{name}_volume_mapping`).
- **Cluster Wiring**: Automatically sets `enable_filestore_csi: true` on GKE clusters.

### `managed-lustre`
Provisions a Google Cloud Managed Lustre parallel file system instance. Recommended for high-bandwidth training datasets, model weights, and checkpoints.
- **Created Modules**:
  - `modules/file-system/managed-lustre`: The Lustre file system instance (`{name}`).
  - `modules/network/private-service-access`: Allocates a `/22` IP range for Lustre communication.
  - `modules/network/firewall-rules`: Opens TCP port 988 for Lustre clients within the VPC.
  - `modules/file-system/gke-persistent-volume`: (GKE only) Provisions K8s PV/PVC.
- **Cluster Wiring**: Automatically enables Lustre CSI driver on GKE.

### `netapp-volume`
Provisions a Google Cloud NetApp Volume inside a dedicated NetApp Storage Pool.
- **Created Modules**:
  - `modules/file-system/netapp-storage-pool`: Storage pool (defaults to 2048 GiB, `FLEX` service level).
  - `modules/file-system/netapp-volume`: Volume instance inside the pool (`{name}`).
  - `modules/network/private-service-access`: Allocates a `/24` range for NetApp private networking.
- **Support**: Available on `slurm` and `jbvm` (unsupported on GKE).

### `cloud-storage`
Provisions a Google Cloud Storage bucket and mounts it to nodes or pods via Cloud Storage FUSE.
- **Created Modules**:
  - `modules/file-system/cloud-storage-bucket`: Storage bucket with hierarchical namespace support (`{name}_bucket`).
  - `modules/file-system/gke-persistent-volume`: (GKE only) Creates K8s PV and PVC using the GCS FUSE CSI driver.

### `aiml-gcsfuse`
Provisions a single GCS bucket and mounts it four times with workload-tuned FUSE flags:
- `/gcs`: General access.
- `/gcs-training-data`: Tuned for high-concurrency read operations.
- `/gcs-checkpoints`: Tuned for large checkpoint write bursts (uses local SSD cache at `/mnt/localssd`).
- `/gcs-model-serving`: Tuned for fast weights loading and read-only cache.
- **Created Modules**:
  - `modules/file-system/cloud-storage-bucket`: Backing GCS bucket (`{name}_bucket`).
  - `modules/file-system/pre-existing-network-storage`: Three additional tuned mount points.
  - `modules/file-system/gke-persistent-volume`: (GKE only) Four PV/PVC pairs.

### `pre-existing-network-storage`
Mounts storage provisioned outside the cluster (existing NFS, Filestore, Lustre, or GCS buckets) without provisioning new cloud resources.
- **Required Inputs**:
  - On `gke`: Exactly one of `gcs_bucket_name`, `filestore_id`, or `lustre_id`.
  - On `slurm` / `jbvm`: `server_ip` and `remote_mount` (unless using `fs_type: gcsfuse`).

### `gke-hyperdisk`
Creates a GKE StorageClass and PersistentVolumeClaim for Google Cloud Hyperdisk (balanced, throughput, or extreme). Disks are dynamically provisioned when pods reference the claim.
- **Created Modules**:
  - `modules/file-system/gke-storage`: Hyperdisk StorageClass and PVC (`{name}`).
- **Key Setting**: `storage_type` (e.g. `Hyperdisk-balanced`, `Hyperdisk-extreme`).

### `kueue-jobset`
Installs the [Kueue](https://kueue.sigs.k8s.io/) job queueing controller and [JobSet](https://jobset.sigs.k8s.io/) multi-node batch controller into your GKE cluster.
- **Created Modules**:
  - `modules/management/kubectl-apply`: Deploys CRDs and controller manifests (`{name}_manager`).
- **Extensibility**: Pass `apply_manifests` under `settings:` to apply arbitrary additional Kubernetes manifests during cluster deployment.

### `gke-job-template`
Generates a Kubernetes Job submission template configured with Workload Identity and node pool selectors, saved to your deployment directory.
- **Created Modules**:
  - `modules/compute/gke-job-template`: Job manifest generator (`{name}`).
- **Requirements**: Requires `image:` under `settings:`, unless populated by a workload overlay (e.g., `nccl-jobset-test`).

### `monitoring-dashboard`
Creates a Google Cloud Monitoring dashboard titled `<deployment_name> Cluster Dashboard` containing pre-built performance charts for CPU, memory, disk I/O, and networking.
- **Created Modules**:
  - `modules/monitoring/dashboard`: Cloud Monitoring dashboard resource (`{name}`).
- **Customization**: Add custom widgets via `widgets:` or select `base_dashboard: Empty` for a blank slate.

### `startup-script`
Injects custom bootstrap logic (shell scripts, Ansible playbooks, or GCS files) into compute nodes, login nodes, or Slurm controllers during boot.
- **Created Modules**:
  - `modules/scripts/startup-script`: Boot script injector (`{name}`).
- **Support**: Available on `slurm` and `jbvm`. Added to the existing node bootstrap chain without overwriting archetype GPU driver setup.

---

## 4. Configuration & Settings Routing

When you specify values under a feature's `settings:` block, `gcluster` uses schema-aware routing to direct inputs to the correct module:

```yaml
features:
  - name: homefs
    type: filestore
    settings:
      filestore_tier: BASIC_SSD             # routes to modules/file-system/filestore
      local_mount: /home                    # routes to modules/file-system/filestore
      connect_mode: PRIVATE_SERVICE_ACCESS  # enables private_service_access module
      prefix_length: 24                     # routes to modules/network/private-service-access
```

### Settings Routing Rules
1. **Primary Module Priority**: Plain settings keys route to the primary module (`{name}` or `{name}_*`) if that module declares them in `variables.tf`.
2. **Sidecar Routing**: If the primary module does not declare an input, the setting routes to any auxiliary module in the feature that accepts it (e.g., `prefix_length` routing to `private_service_access` when `connect_mode: PRIVATE_SERVICE_ACCESS` is enabled).
3. **Submodule Block Targeting**: You can explicitly target an auxiliary module by nesting settings under its submodule key:

   ```yaml
   settings:
     checkpoints:
       local_mount: /my-checkpoints
   ```

4. **Conditional Module Pruning (CEL)**: Features evaluate Common Expression Language (CEL) rules against user settings before building the AST. For example, `private_service_access` is only created when `connect_mode == 'PRIVATE_SERVICE_ACCESS'`.
5. **Fail-Fast Typo Detection**: Any setting key that is unrecognized by all modules in the feature causes an immediate compilation failure with closest-match spelling suggestions.

---

## 5. Interactions & System Wiring

`gcluster` connects features to compute pools and orchestrators automatically using Autowire:

### Storage Auto-Wiring
- **Slurm**: Storage features export their `server_ip`, `remote_mount`, `local_mount`, and `fs_type`. The compiler automatically injects these into `{name}_nodeset` mount configurations and controller epilogs.
- **GKE**: Storage features automatically instantiate `{name}_volume_mapping` (`gke-persistent-volume`), creating matching `PersistentVolume` and `PersistentVolumeClaim` Kubernetes objects so pods can immediately mount the volume.
- **JBVM**: Exports mount specifications directly into the VM instance bootstrap definitions.

### Scoping with `attach_to`
Features accept an optional `attach_to:` list to restrict attachment scope:
- **Workload Feature Duplication**: When a workload feature (such as `gke-job-template`) targets multiple pools, the compiler automatically generates a dedicated job template instance for each targeted pool.

---

## 6. Rules & Constraints

- **Unique Feature Names**: Every item in `features:` must have a unique `name:` within the cluster config. You can instantiate multiple features of the same `type` (e.g., two `filestore` instances) as long as their names differ.
- **Duplicate Mount Detection (Law 7)**: Two storage features cannot use the same `local_mount` path on overlapping compute targets. This includes the default: `filestore`, `managed-lustre` and `netapp-volume` all default to `/shared`, so if you add two of them, set `local_mount` on at least one. If two features share a mount path, compilation fails with an ambiguity error unless their `attach_to` targets are completely disjoint.
- **Orchestrator Compatibility**: Requesting a feature on an unsupported orchestrator (e.g. `kueue-jobset` on `slurm`) triggers a hard error listing compatible orchestrators.
- **Workload Image Requirement**: `gke-job-template` requires an explicit container `image:` in `settings:` unless a workload overlay (e.g. `nccl-jobset-test`) is applied.
