# Create a GKE Cluster and obtain A3 Mega nodes with DWS Flex Start mode

> [!NOTE]
> DWS Flex Start is currently in early development and undergoing extensive testing. While it
> can be used with other machine families, we strongly recommend utilizing it primarily with
> A3 machine families during this phase.

[Dynamic Workload Scheduler](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler) Flex Start mode is designed for fine-tuning models, experimentation, shorter training jobs, distillation, offline inference, and batch jobs.

With Dynamic Workload Scheduler in Flex Start mode, you submit a GPU capacity request for your AI/ML jobs by indicating how many you need, a duration, and your preferred region. It supports capacity requests for up to seven days, with no minimum duration requirement. You can request capacity for as little as a few minutes or hours; typically, the scheduler can fulfill shorter requests more quickly than longer ones.

## DWS Flex Start mode with GKE nodepool and Kueue

**Step 1**: Deploy the blueprint `gke-dws-flex-start.yaml` using the `gcluster` command.

```text
./gcluster deploy examples/gke-dws-flex-start/gke-dws-flex-start.yaml
```

**Step 2**: Connect to the GKE cluster using gcloud command.

```text
gcloud container clusters get-credentials <cluster-name> --location <location> --project <project-name>
```

**Step 3**: Run the sample job.

```text
kubectl apply -f ./examples/gke-dws-flex-start/sample-job.yaml
```

To get details about the job, use

```text
kubectl describe job sample-job
```

## NOTE

1. The `gke-node-pool` module requires these updates.
   - `enable_queued_provisioning` is set to `true`.
   - `autoscaling_total_min_nodes` is set to `0`.
   - `auto_repair` is set to `false`.
   - `auto-upgrade` is set to `false`.
   - Compact placement policy is not supported.
   - Reservations are not supported.

1. The kueue configuration required for DWS Flex Start is included in the dws-queues.yaml file, and can be updated as required.

1. The job resource requests and limits must be aligned with the resources available under ClusterQueue (Kueue resource).

1. The job needs the following additions.
   - Include the label and annotation under the jobset metadata.

   ```yaml
     labels:
       kueue.x-k8s.io/queue-name: {dws kueue name}
     annotations:
       provreq.kueue.x-k8s.io/maxRunDurationSeconds: "7200" # up to 7 days.
   ```

   - Include the nodeSelector under the template spec.

   ```yaml
                 nodeSelector:
                   cloud.google.com/gke-nodepool: {dws nodepool name}
   ```
