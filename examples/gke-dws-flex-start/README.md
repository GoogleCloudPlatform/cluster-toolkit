# Obtaining GKE nodes with DWS Flex

> [!NOTE]
> DWS Flex Start is currently in early development and undergoing extensive testing. While it
> can be used with other machine families, we strongly recommend utilizing it primarily with
> A3 machine families during this phase.

[Dynamic Workload Scheduler](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler) Flex Start mode is designed for fine-tuning models, experimentation, shorter training jobs, distillation, offline inference, and batch jobs.

With Dynamic Workload Scheduler in Flex Start mode, you submit a GPU capacity request for your AI/ML jobs by indicating how many you need, a duration, and your preferred region. It supports capacity requests for up to seven days, with no minimum duration requirement. You can request capacity for as little as a few minutes or hours; typically, the scheduler can fulfill shorter requests more quickly than longer ones.

Cluster Toolkit supports DWS Flex Start mode with GKE nodepool and Kueue.

**Step 1**: Include the following settings in the `gke-node-pool` module.

- `enable_queued_provisioning` is set to `true`.
- `autoscaling_total_min_nodes` is set to `0`.
- `auto_repair` is set to `false`.
- `auto-upgrade` is set to `false`.
- Compact placement policy is not supported.
- Reservations are not supported.

```yaml
  - id: gke_node_pool
    source: modules/compute/gke-node-pool
    use: [gke_cluster, gpunets, gke_service_account]
    settings:
      enable_queued_provisioning: true
      autoscaling_total_min_nodes: 0
      auto_repair: false
      auto_upgrade: false
      # the rest of the settings, e.g. zones, machine_type, etc.
    outputs: [instructions]
```

**Step 2**: Create the Kueue resources for the DWS node pool.

```yaml
apiVersion: kueue.x-k8s.io/v1beta1
kind: ResourceFlavor
metadata:
  name: "default-flavor"
---
apiVersion: kueue.x-k8s.io/v1beta1
kind: AdmissionCheck
metadata:
  name: dws-prov
spec:
  controllerName: kueue.x-k8s.io/provisioning-request
  parameters:
    apiGroup: kueue.x-k8s.io
    kind: ProvisioningRequestConfig
    name: dws-config
---
apiVersion: kueue.x-k8s.io/v1beta1
kind: ProvisioningRequestConfig
metadata:
  name: dws-config
spec:
  provisioningClassName: queued-provisioning.gke.io
  managedResources:
  - nvidia.com/gpu
---
apiVersion: kueue.x-k8s.io/v1beta1
kind: ClusterQueue
metadata:
  name: "dws-cluster-queue"
spec:
  namespaceSelector: {}
  resourceGroups:
  - coveredResources: ["nvidia.com/gpu"]
    flavors:
    - name: "default-flavor"
      resources:
      - name: "nvidia.com/gpu"
        nominalQuota: 512 #  96 nodes
  admissionChecks:
  - dws-prov
---
apiVersion: kueue.x-k8s.io/v1beta1
kind: LocalQueue
metadata:
  namespace: "default"
  name: "dws-local-queue"
spec:
  clusterQueue: "dws-cluster-queue"
--- 
```

**Step 3**: The jobset needs the following additions.  
(a) Include the label and annotation under the jobset metadata.

```yaml
  labels:
    kueue.x-k8s.io/queue-name: {dws kueue name}
  annotations:
    provreq.kueue.x-k8s.io/maxRunDurationSeconds: "7200" # This can probably be up to 7 days.
```

(b) Include the nodeSelector under the template spec.

```yaml
              nodeSelector:
                cloud.google.com/gke-nodepool: {dws nodepool name}
```

> [!NOTE]
> The jobset resource requests and limits must be aligned with the resources under ClusterQueue (Kueue resource).
