# GKE Advanced Infrastructure Features Guide

This guide provides comprehensive documentation on deploying, configuring, and utilizing advanced Google Kubernetes Engine (GKE) hardware and orchestration capabilities in Cluster Toolkit: **Dynamic TPU Slicing**, **Pathways Distributed AI Orchestration**, and **Node Auto-Provisioning (NAP)**.

---

## 1. Dynamic TPU Slicing (TPU v7x and Future Generations)

GKE Dynamic Slicing provides unprecedented flexibility in Tensor Processing Unit (TPU) capacity scheduling by allowing physical hardware blocks to be logically grouped or sliced dynamically on demand. In Cluster Toolkit and GKE, dynamic slicing is supported starting with **TPU v7x (Ironwood) and future TPU generations onwards**. Earlier TPU generations (such as TPU v4, TPU v5e, TPU v5p, and TPU v6e) do not support dynamic slicing and require static slice topologies configured at node pool creation time.

### 1.1 Cluster Blueprint Provisioning & Configuration

To deploy a GKE cluster configured for TPU Dynamic Slicing, configure your TPU v7x blueprint (such as `examples/gke-tpu-7x/gke-tpu-7x-advanced.yaml`) with the following settings:

```yaml
vars:
  machine_type: tpu7x-standard-4t
  accelerator_type: tpu7x
  enable_dynamic_slicing_for_tpus: true
```

#### Key Cluster Configuration Requirements

* **Hardware & Accelerator Type:** Specify a TPU v7x machine type (`tpu7x-standard-4t`) and set `accelerator_type: tpu7x`.
* **Enable Dynamic Slicing Flag:** Set `enable_dynamic_slicing_for_tpus: true` in blueprint `vars`. This deploys the GKE TPU Slice Controller and configures dynamic partition-level topology definitions.
* **Kueue Dynamic Slicing Configuration:** When `enable_dynamic_slicing_for_tpus: true` is set, Cluster Toolkit automatically uses the default dynamic slicing Kueue configuration template (`modules/management/kubectl-apply/kueue/kueue-configuration-dynamic-slicing.yaml.tftpl`), which registers the `tpu-v7x-slice` ResourceFlavor and enables Topology-Aware Scheduling (TAS). Users can optionally override this by passing a custom Kueue template path using `kueue_configuration_path`.

### 1.2 Capabilities & Workload Scheduling (`gcluster job submit`)

* **Elastic Topology Provisioning:** Stitch multiple physical TPU v7x blocks together into a larger logical slice (e.g. connecting multiple `4x4x4` blocks together).
* **Latency Optimization:** Kueue's Topology-Aware Scheduling (TAS) guarantees that TPU pods are placed with minimal network hop latency across the physical TPU interconnect mesh.
* **Automated Scheduling Annotations:** When you submit a job with `--compute-type tpu-v7x-slice` and `--topology <topology>`, Cluster Toolkit automatically translates the request into partition-level requirements (`cloud.google.com/gke-tpu-partition-<topology>-id`) and dynamically switches between single-slice (`kueue.x-k8s.io/podset-required-topology`) and multi-slice (`kueue.x-k8s.io/podset-slice-required-topology`) admission annotation keys based on `--num-slices`.

#### Example CLI Command

Submit a dynamic slicing workload targeting TPU v7x nodes:

```bash
./gcluster job submit \
  --name my-dynamic-slice-job \
  --command "python train.py" \
  --compute-type tpu-v7x-slice \
  --topology 4x4x4 \
  --gke-scheduler gke.io/topology-aware-auto
```

#### GKE Documentation Reference

* [TPU Dynamic Slicing on GKE Concepts](https://cloud.google.com/kubernetes-engine/docs/concepts/tpu-dynamic-slicing)
* [Scheduling Dynamic Slices with Kueue and TAS on GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/kueue-tpu-dynamic-slicing)

---

## 2. Pathways Distributed AI Orchestration

Pathways is Google's specialized distributed AI execution framework designed to coordinate large-scale multi-slice TPU machine learning workloads. Cluster Toolkit provides native integration for compiling and deploying Pathways-enabled workloads without manual multi-job Kubernetes manifest configuration.

### 2.1 Cluster Blueprint Provisioning & Configuration

To prepare a GKE cluster for Pathways execution, configure your cluster blueprint with the following structural components:

```yaml
vars:
  enable_pathways_for_tpus: true
  kueue_configuration_path: $(ghpc_stage("./kueue-configuration.yaml.tftpl"))
```

#### Key Cluster Configuration Requirements

* **Dedicated CPU Coordinator Node Pool:** Pathways relies on CPU-based Resource Manager (`pathways-rm`) and Proxy (`pathways-proxy`) services to coordinate multi-slice TPU execution. Ensure your blueprint includes a system or CPU compute node pool (e.g., `n2-standard-32`) so coordinator pods are scheduled on CPU nodes rather than consuming expensive TPU chips.
* **Enable Pathways Flag:** Set `enable_pathways_for_tpus: true` in blueprint `vars`. This configures Kueue ClusterQueues and LocalQueues with multi-slice resource quotas tailored for Pathways.
* **Kueue Pathways Manifest:** Link a Kueue template configured for Pathways (`kueue-configuration-pathways.yaml.tftpl` or `kueue-configuration-dynamic-slicing-pathways.yaml.tftpl`).
* **IAM & Workload Identity Permissions:** If using state persistence (`export ENABLE_PATHWAYS_PERSISTENCE='1'`), ensure the Google Cloud Service Account (GSA) associated with your workload (typically suffixed with `gke-wl-sa`) is granted `storage.admin` or `storage.objectAdmin` roles on your Google Cloud Storage bucket.

### 2.2 Workload Orchestration Roles & Scheduling (`gcluster job submit`)

When `--pathways` is specified during job submission, Cluster Toolkit automatically refactors the Kubernetes JobSet manifest to deploy and coordinate three distinct functional roles:

1. **Pathways ResourceManager / Server (`pathways-rm`):** Deployed within the coordinator job to manage dynamic TPU worker allocations and host mappings.
2. **Pathways Proxy (`pathways-proxy`):** Handles execution requests and acts as the entry point for the client workload.
3. **Co-Located JAX Workers & Sidecars:** Deployed as worker pods hosting JAX and PJRT runtimes across independent TPU slices, with optional Python sidecar containers injected via `--pathways-colocated-python-sidecar-image`.

#### Headless Pathways Mode (`--pathways-headless`)

When `--pathways-headless` is enabled, Cluster Toolkit deploys the Pathways infrastructure without running an in-cluster client workload container:

* **No Workload Container:** The `--image`, `--base-image`, and `--command` flags are **not required**.
* **External Client Connection:** Connect to the running Pathways cluster externally (e.g. from a local notebook or Vertex AI development instance) by port-forwarding the proxy server container port `29000`:

  ```bash
  kubectl port-forward <pathways-head-pod> 29000:29000
  ```

  And initializing your JAX/Pathways client against `grpc://127.0.0.1:29000`.

#### Example CLI Commands

Submit a standard multi-slice Pathways distributed training job:

```bash
./gcluster job submit \
  --name my-pathways-job \
  --command "python train_pathways.py" \
  --compute-type v6e-16 \
  --pathways \
  --pathways-gcs-location gs://<YOUR_BUCKET_NAME>/pathways-artifacts
```

Submit a headless Pathways cluster infrastructure for external client connections:

```bash
./gcluster job submit \
  --name my-pathways-headless \
  --compute-type v6e-16 \
  --pathways \
  --pathways-gcs-location gs://<YOUR_BUCKET_NAME>/pathways-artifacts \
  --pathways-headless
```

---

## 3. Node Auto-Provisioning (NAP) and Compute Consumption

Node Auto-Provisioning (NAP) is a GKE cluster-level autoscaling capability that dynamically creates, manages, and deletes node pools based on unschedulable pod resource requirements. Rather than pre-provisioning static node pools, NAP allows your cluster to scale compute resources on demand.

### 3.1 Cluster Blueprint Provisioning & Configuration

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

#### Key Cluster Configuration Requirements

* **Resource Limits:** Specify `minimum` and `maximum` bounds for CPU, memory, and accelerator types. NAP will only spin up node pools whose total aggregate consumption stays within these defined bounds.
* **Kueue Resource Quota Alignment:** When integrating with Kueue for job queuing, ensure that Kueue ClusterQueue nominal capacities correspond to your GKE NAP maximum resource bounds so Kueue can admit workloads smoothly ahead of NAP node pool creation.

### 3.2 Job Submission & Workload Scheduling (`gcluster job submit`)

When submitting jobs to a NAP-enabled GKE cluster via `gcluster job submit`, you can target specific compute consumption models without modifying Kubernetes manifests manually:

* **Spot vs. On-Demand Provisioning:** Use `--gke-nap-provisioning spot` or `--gke-nap-provisioning on-demand`. When `spot` is specified, Cluster Toolkit injects the standard GKE provisioning toleration (`cloud.google.com/gke-provisioning=spot:NoSchedule`) and node selector into the pod template.
* **GCE Reservation Targeting:** Use `--gke-nap-provisioning reservation` in combination with `--gke-nap-reservation <reservation-name>`. Cluster Toolkit automatically populates the reservation node selector and tolerations (`cloud.google.com/reservation-name=<reservation-name>:NoSchedule`), enabling GKE NAP to spawn node pools directly inside your targeted GCE reservation. You can also pass a full GCP resource URI (e.g., `projects/<project-id>/reservations/<reservation-name>`) to target shared reservations in other projects, which automatically configures the `cloud.google.com/reservation-project` label.
* **Pre-Flight Limit Verification:** Before submitting a job, Cluster Toolkit queries GKE cluster metadata to verify that the requested machine type (e.g. `v6e-4`, `a3-megagpu-8g`) is explicitly permitted by your cluster's NAP resource limits. If not permitted, submission fails fast with a clear diagnostic error.

#### Example CLI Commands

Target Spot VMs via GKE Node Auto-Provisioning:

```bash
./gcluster job submit \
  --name my-nap-spot-job \
  --command "python app.py" \
  --compute-type v6e-4 \
  --gke-nap-provisioning spot
```

Target a GCE reservation via GKE Node Auto-Provisioning:

```bash
./gcluster job submit \
  --name my-nap-reservation-job \
  --command "python app.py" \
  --compute-type v6e-4 \
  --gke-nap-provisioning reservation \
  --gke-nap-reservation my-tpu-reservation
```
