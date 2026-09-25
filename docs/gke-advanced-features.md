# Guide to advanced GKE infrastructure features

Cluster Toolkit simplifies deploying and orchestrating high-performance AI and machine learning workloads on Google Kubernetes Engine (GKE). By configuring dynamic TPU slicing, Pathways distributed AI orchestration, and Node Auto-Provisioning (NAP), you can elastically scale compute capacity, coordinate multi-slice training jobs, and optimize resource costs across Spot VMs and Compute Engine reservations.

---

## 1. Dynamic TPU slicing (TPU v7x and future generations)

GKE Dynamic Slicing provides flexible Tensor Processing Unit (TPU) capacity scheduling by letting you logically group or slice physical hardware cubes dynamically. In Cluster Toolkit and GKE, dynamic slicing is supported starting with TPU v7x (Ironwood) and future TPU generations. Earlier TPU generations (such as TPU v4, TPU v5e, TPU v5p, and TPU v6e) do not support dynamic slicing and require static slice topologies configured at node pool creation time. Dynamic slicing supports both superslicing (aggregating multiple physical cubes into larger topologies) and subslicing (partitioning a single cube into smaller fractional topologies) dynamically at job submission time.

### 1.1 Cluster blueprint provisioning and configuration

To deploy a GKE cluster configured for TPU Dynamic Slicing, configure your TPU v7x blueprint (such as the `examples/gke-tpu-7x/gke-tpu-7x-advanced.yaml` file) with the following settings:

```yaml
vars:
  machine_type: tpu7x-standard-4t
  accelerator_type: tpu7x
  enable_dynamic_slicing_for_tpus: true
```

#### Key cluster configuration requirements

Configuring dynamic slicing requires the following settings:

* **Hardware and accelerator type:** Specify a TPU v7x machine type (`tpu7x-standard-4t`) and set `accelerator_type: tpu7x`.
* **Enable dynamic slicing:** Set `enable_dynamic_slicing_for_tpus: true` in the `vars` block. This setting deploys the GKE TPU Slice Controller, automatically configures the `workload_policy` module's `accelerator_topology_mode` to `PROVISION_ONLY` (or `null` when disabled, unless explicitly overridden in `workload_policy`), and configures dynamic partition-level topology definitions.
* **Kueue dynamic slicing configuration:** When you set `enable_dynamic_slicing_for_tpus: true`, Cluster Toolkit automatically uses the default dynamic slicing Kueue configuration template (the `modules/management/kubectl-apply/kueue/kueue-configuration-dynamic-slicing.yaml.tftpl` file, or `kueue-configuration-dynamic-slicing-pathways.yaml.tftpl` when Pathways is also enabled), which registers the `tpu-flavor` (or `flavor-tpu7x`) `ResourceFlavor` targeting `cloud.google.com/gke-tpu-accelerator: tpu7x` and enables Topology-Aware Scheduling (TAS). You can optionally override this setting by passing a custom template path using `kueue.config_path` in the `kubectl-apply` module settings.

> **Note:** Switching an existing cluster between static slicing and dynamic slicing (`enable_dynamic_slicing_for_tpus`) changes `workload_policy.accelerator_topology_mode`, which is an immutable field on the Compute Engine resource policy (`ForceNew`). Because Compute Engine prevents replacing a resource policy while it is attached to an active node pool, toggling this setting on a running cluster requires recreating the TPU node pool and resource policy.

### 1.2 Capabilities and workload scheduling (`gcluster job submit`)

Workload scheduling with dynamic slicing provides the following capabilities:

* **Dynamic superslicing:** Aggregate multiple physical TPU v7x cubes together into a larger logical slice (such as combining multiple `4x4x4` cubes into `4x4x8` or `4x4x16` topologies) dynamically for large-scale distributed training.
* **Dynamic subslicing:** Partition a single physical TPU cube into smaller fractional topologies (such as slicing a `4x4x4` cube into `2x2x4` or `2x4x4` sub-slices) dynamically, enabling efficient bin-packing and co-tenancy for smaller workloads.
* **Latency optimization:** Kueue Topology-Aware Scheduling (TAS) places TPU pods with minimal network hop latency across the physical TPU interconnect mesh.
* **Automated scheduling annotations:** When you submit a job with `--compute-type tpu7x` and `--topology TOPOLOGY`, Cluster Toolkit automatically injects the `cloud.google.com/gke-tpu-slice-topology` Pod annotation, translates the request into partition-level requirements (`cloud.google.com/gke-tpu-partition-TOPOLOGY-id`), and dynamically switches between single-slice (`kueue.x-k8s.io/podset-required-topology`) and multi-slice (`kueue.x-k8s.io/podset-slice-required-topology` along with `kueue.x-k8s.io/podset-slice-size`) admission annotation keys based on `--num-slices`.

#### Example CLI command

Submit a dynamic slicing workload that requests a `4x4x4` TPU v7x topology:

```shell
./gcluster job submit \
  --name my-dynamic-slice-job \
  --command "python train.py" \
  --compute-type tpu7x \
  --topology 4x4x4
```

#### GKE documentation reference

* [TPU Dynamic Slicing on GKE Concepts](https://cloud.google.com/kubernetes-engine/docs/concepts/tpu-dynamic-slicing)
* [Scheduling Dynamic Slices with Kueue and TAS on GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/kueue-tpu-dynamic-slicing)

---

## 2. Pathways distributed AI orchestration

Pathways is a specialized distributed AI execution framework designed to coordinate large-scale multi-slice TPU machine learning workloads. Cluster Toolkit provides native integration for compiling and deploying Pathways-enabled workloads without requiring manual multi-job Kubernetes manifest configuration.

### 2.1 Cluster blueprint provisioning and configuration

To prepare a GKE cluster for Pathways execution, configure your cluster blueprint with the following structural components:

```yaml
vars:
  enable_pathways_for_tpus: true
```

#### Key cluster configuration requirements

Pathways cluster configuration requires the following components:

* **Dedicated CPU coordinator node pool:** Pathways relies on CPU-based Resource Manager (`pathways-rm`) and Proxy (`pathways-proxy`) services to coordinate multi-slice TPU execution. When you set `enable_pathways_for_tpus: true` on the `gke-cluster` module, Cluster Toolkit automatically creates an autoscaled CPU node pool named `cpu-np` (`n4-standard-64`) so that coordinator pods are scheduled on CPU nodes rather than consuming TPU chips (you can also target a custom CPU node pool at job submission time using `--pathways-head-np`).
* **Enable Pathways flag:** Set `enable_pathways_for_tpus: true` in the `vars` block. This setting configures Kueue ClusterQueues and LocalQueues with multi-slice resource quotas tailored for Pathways.
* **Kueue Pathways configuration:** When you set `enable_pathways_for_tpus: true`, Cluster Toolkit automatically uses the default Pathways Kueue configuration template (the `modules/management/kubectl-apply/kueue/kueue-configuration-pathways.yaml.tftpl` file, or the `kueue-configuration-dynamic-slicing-pathways.yaml.tftpl` file if dynamic slicing is also enabled). You can optionally override this setting by passing a custom template path using `kueue.config_path` in the `kubectl-apply` module settings.
* **Unified Kueue resource groups and quotas:** When Pathways is active, Cluster Toolkit programmatically unifies Kueue ClusterQueue resource groups (`["google.com/tpu", "cpu", "memory"]`) into a single unified resource group. This setting prevents scheduling conflicts and node selector merging issues on TPU worker pods that request both TPU and CPU or memory resources. ClusterQueue nominal quotas (`tpu_flavor_cpu_quota`, `tpu_flavor_memory_quota`, `tpu_quota`) automatically scale to the physical hardware capacity of your cluster, defaulting to high limits to prevent bottlenecks while supporting custom overrides by using the `config_template_vars` variable.
* **IAM and Workload Identity permissions:** If you use state persistence (`export ENABLE_PATHWAYS_PERSISTENCE='1'`), ensure that the Google Cloud Service Account (GSA) associated with your workload (typically suffixed with `gke-wl-sa`) is granted the `storage.admin` or `storage.objectAdmin` role on your Cloud Storage bucket.

### 2.2 Workload orchestration roles and scheduling (`gcluster job submit`)

When you specify the `--pathways` flag during job submission, Cluster Toolkit automatically refactors the Kubernetes JobSet manifest to deploy and coordinate three distinct functional roles:

1. **Pathways ResourceManager server (`pathways-rm`):** Deployed within the coordinator job to manage dynamic TPU worker allocations and host mappings.
2. **Pathways Proxy (`pathways-proxy`):** Handles execution requests and acts as the entry point for the client workload.
3. **Co-located JAX workers and sidecars:** Deployed as worker pods hosting JAX and PJRT runtimes across independent TPU slices, with optional Python sidecar containers injected by using the `--pathways-colocated-python-sidecar-image` flag.

#### Headless Pathways mode (`--pathways-headless`)

When you enable the `--pathways-headless` flag, Cluster Toolkit deploys the Pathways infrastructure without running an in-cluster client workload container:

* **No workload container:** The `--image`, `--base-image`, and `--command` flags are not required.
* **External client connection:** Connect to the running Pathways cluster externally (for example, from a local notebook or Vertex AI development instance) by port-forwarding the proxy server container port `29000`:

  Forward local port `29000` to the running Pathways proxy server:

  ```shell
  kubectl port-forward PATHWAYS_HEAD_POD 29000:29000
  ```

  Then initialize your JAX/Pathways client against `grpc://127.0.0.1:29000`.

#### Example CLI commands

Submit a multi-slice Pathways training workload with Cloud Storage state persistence:

```shell
./gcluster job submit \
  --name my-pathways-job \
  --command "python train_pathways.py" \
  --compute-type v6e-16 \
  --pathways \
  --pathways-gcs-location gs://BUCKET_NAME/pathways-artifacts
```

Deploy a headless Pathways server cluster for interactive client connections:

```shell
./gcluster job submit \
  --name my-pathways-headless \
  --compute-type v6e-16 \
  --pathways \
  --pathways-gcs-location gs://BUCKET_NAME/pathways-artifacts \
  --pathways-headless
```

---

## 3. Node auto-provisioning (NAP) and compute consumption

Node Auto-Provisioning (NAP) is a GKE cluster-level autoscaling capability that dynamically creates, manages, and deletes node pools based on unschedulable pod resource requirements. Rather than pre-provisioning static node pools, NAP lets your cluster scale compute resources dynamically based on incoming workload requirements.

### 3.1 Cluster blueprint provisioning and configuration

To enable Node Auto-Provisioning on your GKE cluster, configure the `gke-cluster` module in your Cluster Toolkit blueprint with `cluster_autoscaling` settings:

```yaml
deployment_groups:
- group: primary
  modules:
  - id: my-gke-cluster
    source: modules/scheduler/gke-cluster
    settings:
      cluster_autoscaling:
        enabled: true
        autoprovisioning_disk_size_gb: 100
        autoprovisioning_disk_type: pd-balanced
        autoprovisioning_cpu_max: 1000
        autoprovisioning_memory_max: 4000
        limits:
          - autoprovisioning_machine_type: g2-standard-48
            autoprovisioning_max_count: 64
          - autoprovisioning_machine_type: ct6e-standard-4t
            autoprovisioning_max_count: 16
```

#### Key cluster configuration requirements

NAP cluster configuration requires the following settings:

* **Resource limits:** Specify `autoprovisioning_cpu_max`, `autoprovisioning_memory_max`, and `limits` (`autoprovisioning_machine_type` and `autoprovisioning_max_count`). NAP only creates node pools whose aggregate consumption stays within these defined bounds.
* **Kueue resource quota alignment:** When integrating with Kueue for job queuing, ensure that Kueue ClusterQueue nominal capacities correspond to your GKE NAP maximum resource bounds so that Kueue can admit workloads smoothly ahead of NAP node pool creation.

### 3.2 Job submission and workload scheduling (`gcluster job submit`)

> [!NOTE]
> When running on a NAP-enabled cluster, standard job submissions without `--gke-nap-*` flags automatically trigger on-demand node pool creation if existing nodes lack sufficient capacity. The `--gke-nap-provisioning` and `--gke-nap-reservation` flags are only required when you want to target **Spot VMs** or specific **Compute Engine reservations**.
>
> Cluster Toolkit currently supports **On-Demand**, **Spot**, and **Reservation** models with Node Auto-Provisioning. Dynamic Workload Scheduler (DWS) Flex-Start and Queued Provisioning are supported via static cluster blueprints.

When submitting jobs to a NAP-enabled GKE cluster by using the `gcluster job submit` command, you can target specific compute consumption models without modifying Kubernetes manifests manually:

* **Spot versus on-demand provisioning:** Use `--gke-nap-provisioning spot` or `--gke-nap-provisioning on-demand`. When you specify `spot`, Cluster Toolkit injects the standard GKE provisioning toleration (`cloud.google.com/gke-provisioning=spot:NoSchedule`) and node selector into the pod template.
* **Compute Engine reservation targeting:** Use `--gke-nap-provisioning reservation` in combination with `--gke-nap-reservation RESERVATION_NAME`. Cluster Toolkit automatically populates the reservation node selector and tolerations (`cloud.google.com/reservation-name=RESERVATION_NAME:NoSchedule`), letting GKE NAP spawn node pools directly inside your targeted Compute Engine reservation. You can also pass a full Google Cloud resource URI (for example, `projects/PROJECT_ID/reservations/RESERVATION_NAME`) to target shared reservations in other projects, which automatically configures the `cloud.google.com/reservation-project` label.
* **Pre-flight limit verification:** Before submitting a job, Cluster Toolkit queries GKE cluster metadata to verify that the requested machine type (for example, `v6e-4` or `a3-megagpu-8g`) is explicitly permitted by your cluster's NAP resource limits. If not permitted, submission fails quickly with a clear diagnostic error.

#### Example CLI commands

Submit a workload that provisions Spot VMs dynamically:

```shell
./gcluster job submit \
  --name my-nap-spot-job \
  --command "python app.py" \
  --compute-type v6e-4 \
  --gke-nap-provisioning spot
```

Submit a workload that targets an existing Compute Engine reservation:

```shell
./gcluster job submit \
  --name my-nap-reservation-job \
  --command "python app.py" \
  --compute-type v6e-4 \
  --gke-nap-provisioning reservation \
  --gke-nap-reservation RESERVATION_NAME
```

---

## 4. Multi-Tier Checkpointing (MTC / HighScaleCheckpointing)

Multi-Tier Checkpointing (MTC) provides high-throughput, low-latency checkpoint saving and restoration for large-scale LLM and AI training jobs on GKE. MTC buffers checkpoints in local node memory (RAM / local SSD) for ultra-fast saves, while asynchronously background-replicating checkpoint files to Google Cloud Storage (GCS).

### 4.1 Blueprint configuration

To enable MTC on your GKE cluster, configure `enable_multi_tier_checkpointing: true` in your `gke-cluster` module settings, grant GKE Node Service Account access in your GCS checkpoint bucket's `object_users` list, and apply the `CheckpointConfiguration` Custom Resource (CR):

```yaml
deployment_groups:
- group: primary
  modules:
  - id: checkpoint_bucket
    source: modules/file-system/cloud-storage-bucket
    settings:
      # Pass the GKE Node Service Account to grant GCS read/write/list object permissions:
      object_users:
        gke_node_sa: "serviceAccount:$(gke_cluster.node_service_account)"

  - id: gke_cluster
    source: modules/scheduler/gke-cluster
    settings:
      enable_multi_tier_checkpointing: true  # Enables MTC GKE Addon & Workload Identity

  - id: apply_mtc_config
    source: modules/management/kubectl-apply
    use: [gke_cluster]
    settings:
      apply_manifests:
      - name: checkpoint-configuration
        source: $(ghpc_stage("../modules/management/kubectl-apply/manifests/checkpoint-configuration.yaml.tftpl"))
        template_vars:
          namespace: "default"
          inMemoryVolumeSize: "50Gi"
          cloudStorageBucketName: $(checkpoint_bucket.gcs_bucket_name)
```

#### Key configuration notes
* **Workload Identity & IAM Permissions**: Setting `enable_multi_tier_checkpointing: true` automatically binds the MTC Kubernetes Service Account (`gke-managed-checkpointing/gke-checkpointing-multitier-node`) to the Node GCP Service Account via Workload Identity.
* **GCS Bucket Permissions**: Ensure the `checkpoint_bucket` includes `object_users: { gke_node_sa: "serviceAccount:$(gke_cluster.node_service_account)" }` so the MTC background replicator daemonset has object write, list, and read access (`roles/storage.objectUser`).

### 4.2 One-Command MTC Job Submission (`gcluster job submit --gke-mtc-enabled`)

When submitting training workloads via `gcluster job submit`, add the `--gke-mtc-enabled` flag. `gcluster` will automatically inject the MTC CSI driver (`multitier-checkpoint.csi.storage.gke.io`) into your pod specification mounted at `/tmp/mtc_checkpoints`:

```shell
./gcluster job submit \
  --name my-mtc-training-job \
  --command "python train.py --checkpoint-dir=/tmp/mtc_checkpoints" \
  --compute-type v6e-4 \
  --gke-mtc-enabled
```

---

## 5. Cloud Storage FUSE storage profiles

[GKE Cloud Storage FUSE storage profiles](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gcsfuse-profiles) tune GCSFuse automatically for AI/ML access patterns. Adding `profile=` to a `gs://` mount in `gcluster job submit` generates the PersistentVolume and PersistentVolumeClaim ("gateway") the profile needs, instead of an inline CSI volume. The `--mount` format itself is described in the [gcluster job guide](gcluster_job_guide.md#44-example-submit-job-with-persistent-storage).

### 5.1 Cluster prerequisites

* **GKE version**: GKE `1.35.1-gke.1616000` or later with the Cloud Storage FUSE CSI driver enabled (`enable_gcsfuse_csi: true` on the `gke-cluster` module). Verify with `kubectl get sc -l gke-gcsfuse/profile=true`, which lists the three StorageClasses.
* **GKE Service Agent IAM**: the GKE Service Agent (`service-<PROJECT_NUMBER>@container-engine-robot.iam.gserviceaccount.com`) needs bucket permissions to scan the bucket and, for Rapid Cache, to manage caches (see [Configure IAM permissions](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gcsfuse-profiles#configure_permissions)). Buckets provisioned through the [`gke-persistent-volume`](../modules/file-system/gke-persistent-volume/README.md) module get this binding automatically.

> [!IMPORTANT]
> **`profile=serving` requires the bucket and the cluster to be in the same
> region.** GKE documents this co-location as *mandatory* for the
> `gcsfusecsi-serving` profile, and equally mandatory whenever Rapid Cache
> (`anywhereCacheZones`) is enabled - on any profile.
>
> Confirm the bucket's location before submitting:
>
> ```bash
> gcloud storage buckets describe gs://<YOUR_BUCKET_NAME> --format="value(location)"
> ```
>
> `training` and `checkpointing` without Rapid Cache do not carry this hard
> requirement, though same-region buckets remain the better choice for
> throughput and egress cost.

### 5.2 Job submission (`gcluster job submit --mount "...;profile=<profile>"`)

Accepted profiles are `training`, `checkpointing`, and `serving` (canonical `gcsfusecsi-<name>` names are also accepted). To choose one, see [Select performance profile](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gcsfuse-profiles#select-performance-profile).

```shell
./gcluster job submit \
  --name my-serving-job \
  --command "python serve.py" \
  --compute-type n2-standard-32 \
  --image us-docker.pkg.dev/my-project/my-repo/my-image:latest \
  --mount "gs://<YOUR_MODEL_BUCKET>/llama;/models;ro;profile=serving"
```

Before applying anything, `gcluster` checks each profile mount:

* A StorageClass missing from the cluster fails the submission.
* A bucket outside the cluster's region fails the submission where GKE requires co-location (`serving`, or any profile with Rapid Cache enabled), and only warns otherwise.
* A GKE Service Agent that appears to lack the bucket permissions produces a warning; submission continues.

A dry run (`--dry-run-out`) turns the blocking checks into warnings.

#### Volume attributes

`attributes=<k=v,...>` overrides the [storage profile StorageClass parameters](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gcsfuse-profiles#storageclass_configuration_reference) on the generated volume (without `profile=`, it sets [GCSFuse CSI volume attributes](https://docs.cloud.google.com/kubernetes-engine/docs/reference/cloud-storage-fuse-csi-driver/volume-attr) on the inline mount). `gcluster` derives some attributes from the rest of the `--mount` spec, and supplying one of them through `attributes=` fails validation, so the two cannot silently overwrite each other. Use `options=<opt1>,<opt2>` instead of `attributes=mountOptions=...` for gcsfuse mount flags, and `src=gs://<bucket>` instead of `attributes=bucketName=...` for the bucket.

#### Gateway sharing and naming

* The generated claim is named `gcluster-gcsfuse-<bucket>-<profile>-<hash>` and the
  PersistentVolume `gcluster-gcsfuse-<bucket>-<profile>-<hash>-<namespace>`, where
  `<hash>` is a short digest of the volume's settings. The names are
  deterministic, so several jobs that use the same bucket, profile, and settings
  in the same namespace **share one gateway** rather than each creating their own.
  A bucket subpath is applied on the pod's mount, so it does not create a new gateway.
* Any change to the settings gives the mount its own gateway, so
  it never clashes with the immutable spec of an existing one. Running jobs keep
  using the old gateway.

#### Managing gateways

`gcluster job cancel` cleans up gateways automatically: after deleting the job, it
deletes every gateway claim in the namespace that no other workload still uses,
along with the PersistentVolume bound to it. Gateways created in the last two
minutes are kept, so a job being submitted at the same time does not lose its
volume. Bucket contents are never affected.

Cleanup runs only on `gcluster job cancel`. A gateway is left behind if its job
finished on its own or was deleted with `kubectl`. Running `gcluster job cancel`
for any job in that namespace later removes it. To list gateways:

```bash
kubectl get pv,pvc -A -l gcluster.google.com/managed-by=cluster-toolkit,gcluster.google.com/storage-type=gcsfuse
```

To delete one by hand, delete the claim first, then the volume bound to it:

```bash
kubectl get pvc <claim> -n <namespace> -o jsonpath='{.spec.volumeName}'
kubectl delete pvc <claim> -n <namespace>
kubectl delete pv <volume>
```

---

## What's next

* [TPU Dynamic Slicing on GKE Concepts](https://cloud.google.com/kubernetes-engine/docs/concepts/tpu-dynamic-slicing)
* [Scheduling Dynamic Slices with Kueue and TAS on GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/kueue-tpu-dynamic-slicing)
* [GKE Node Auto-Provisioning Documentation](https://cloud.google.com/kubernetes-engine/docs/concepts/node-auto-provisioning)
* [Multi-Tier Checkpointing on GKE Overview](https://cloud.google.com/kubernetes-engine/docs/concepts/multi-tier-checkpointing)
* [Cloud Storage FUSE storage profiles on GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gcsfuse-profiles)
