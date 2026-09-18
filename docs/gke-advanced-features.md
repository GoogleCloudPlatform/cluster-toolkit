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
* **Enable dynamic slicing flag:** Set `enable_dynamic_slicing_for_tpus: true` in the `vars` block. This setting deploys the GKE TPU Slice Controller and configures dynamic partition-level topology definitions.
* **Kueue dynamic slicing configuration:** When you set `enable_dynamic_slicing_for_tpus: true`, Cluster Toolkit automatically uses the default dynamic slicing Kueue configuration template (the `modules/management/kubectl-apply/kueue/kueue-configuration-dynamic-slicing.yaml.tftpl` file), which registers the `tpu-v7x-slice` ResourceFlavor and enables Topology-Aware Scheduling (TAS). You can optionally override this setting by passing a custom template path using the `kueue_configuration_path` variable.

### 1.2 Capabilities and workload scheduling (`gcluster job submit`)

Workload scheduling with dynamic slicing provides the following capabilities:

* **Dynamic superslicing:** Aggregate multiple physical TPU v7x cubes together into a larger logical slice (such as combining multiple `4x4x4` cubes into `4x4x8` or `4x4x16` topologies) dynamically for large-scale distributed training.
* **Dynamic subslicing:** Partition a single physical TPU cube into smaller fractional topologies (such as slicing a `4x4x4` cube into `2x2x4` or `2x4x4` sub-slices) dynamically, enabling efficient bin-packing and co-tenancy for smaller workloads.
* **Latency optimization:** Kueue Topology-Aware Scheduling (TAS) places TPU pods with minimal network hop latency across the physical TPU interconnect mesh.
* **Automated scheduling annotations:** When you submit a job with `--compute-type tpu-v7x-slice` and `--topology TOPOLOGY`, Cluster Toolkit automatically translates the request into partition-level requirements (`cloud.google.com/gke-tpu-partition-TOPOLOGY-id`) and dynamically switches between single-slice (`kueue.x-k8s.io/podset-required-topology`) and multi-slice (`kueue.x-k8s.io/podset-slice-required-topology`) admission annotation keys based on `--num-slices`.

#### Example CLI command

Submit a dynamic slicing workload that requests a `4x4x4` TPU v7x topology:

```shell
./gcluster job submit \
  --name my-dynamic-slice-job \
  --command "python train.py" \
  --compute-type tpu-v7x-slice \
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

* **Dedicated CPU coordinator node pool:** Pathways relies on CPU-based Resource Manager (`pathways-rm`) and Proxy (`pathways-proxy`) services to coordinate multi-slice TPU execution. Ensure that your blueprint includes a system or CPU compute node pool (for example, `n2-standard-32`) so that coordinator pods are scheduled on CPU nodes rather than consuming TPU chips.
* **Enable Pathways flag:** Set `enable_pathways_for_tpus: true` in the `vars` block. This setting configures Kueue ClusterQueues and LocalQueues with multi-slice resource quotas tailored for Pathways.
* **Kueue Pathways configuration:** When you set `enable_pathways_for_tpus: true`, Cluster Toolkit automatically uses the default Pathways Kueue configuration template (the `modules/management/kubectl-apply/kueue/kueue-configuration-pathways.yaml.tftpl` file, or the `kueue-configuration-dynamic-slicing-pathways.yaml.tftpl` file if dynamic slicing is also enabled). You can optionally override this setting by passing a custom template path using the `kueue_configuration_path` variable.
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
        autoscaling_profile: OPTIMIZE_UTILIZATION # or BALANCED
        resource_limits:
          - resource_type: cpu
            minimum: 1
            maximum: 1000
          - resource_type: memory
            minimum: 1
            maximum: 4000
          - resource_type: nvidia-l4
            minimum: 0
            maximum: 64
```

#### Key cluster configuration requirements

NAP cluster configuration requires the following settings:

* **Resource limits:** Specify `minimum` and `maximum` bounds for CPU, memory, and accelerator types. NAP only creates node pools whose aggregate consumption stays within these defined bounds.
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

## What's next

* [TPU Dynamic Slicing on GKE Concepts](https://cloud.google.com/kubernetes-engine/docs/concepts/tpu-dynamic-slicing)
* [Scheduling Dynamic Slices with Kueue and TAS on GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/kueue-tpu-dynamic-slicing)
* [GKE Node Auto-Provisioning Documentation](https://cloud.google.com/kubernetes-engine/docs/concepts/node-auto-provisioning)
