# Deploy an A3 Edge GKE cluster for inference

This blueprint creates a Google Kubernetes Engine (GKE) cluster with A3 Edge
nodes (`a3-edgegpu-8g`) for serving LLM inference. Each A3 Edge VM has 8 NVIDIA
H100 80GB GPUs, 16 Local NVMe SSDs and 4 x 100 Gbps GPU network interfaces.

The blueprint sets up:

- **GKE Inference Gateway**: Gateway API with the `InferencePool` /
  `InferenceObjective` CRDs and a regional proxy-only subnet, ready for a model
  server plus Endpoint Picker (EPP) deployment.
- **Local SSD** on the GPU nodes, available to pods as `emptyDir`, for KV-cache
  offload and scratch space.
- **Cloud Storage FUSE CSI driver** with Workload Identity, for loading model
  weights from GCS buckets.
- **CPU node pool** (`n2d-standard-2`, autoscaling 1-3) for non-GPU components
  such as the Inference Gateway Endpoint Picker.
- **GPUDirect-TCPX networking** (4 GPU VPCs and the TCPX NCCL plugin) for
  multi-node serving.
- **Kueue and JobSet** for running batch jobs on the GPU nodes.

## Prerequisites

1. **Cluster Toolkit**: install the
   [dependencies](https://docs.cloud.google.com/cluster-toolkit/docs/setup/install-dependencies)
   and [set up the toolkit](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment).
2. **Zone**: `a3-edgegpu-8g` is offered only in a limited set of zones (for
   example `asia-south1-c`, `asia-northeast3-c`, `europe-west2-b`). The GPU
   node pool is zonal, so your reservation and the `zone` variable must point
   at the same zone.
3. **Quota** in the chosen region, per A3 Edge node:

   | Quota | Per node |
   | :--- | :--- |
   | `NVIDIA_H100_GPUS` (or committed/reservation equivalent) | 8 |
   | `A3_CPUS` | 208 |
   | `LOCAL_SSD_TOTAL_GB` | 6,000 |

   You also need quota for the CPU node pools (`N2D_CPUS`, and `N2_CPUS` or
   `N4_CPUS` depending on what is available in the zone).
4. **APIs**: the Inference Gateway needs the Network Services API in addition to
   Compute and GKE. Without it the Gateway stays `Programmed=False`.

   ```bash
   gcloud services enable container.googleapis.com compute.googleapis.com \
     networkservices.googleapis.com --project <PROJECT_ID>
   ```

5. **IP address**: the public IP of the machine running `gcluster`.
6. **GCS bucket** for Terraform state.

## Configuration

Fill out `gke-a3-edgegpu-inference-deployment.yaml` with your values:

| Variable | Description |
| :--- | :--- |
| `project_id` | Your Google Cloud Project ID. |
| `deployment_name` | A unique name for this deployment. Also used as the GKE cluster name. |
| `region` / `zone` | Region and zone of the GPU node pool, for example `asia-south1` / `asia-south1-c`. |
| `authorized_cidr` | Public IP of the machine running `gcluster`, in CIDR notation (`1.2.3.4/32`). |
| `bucket` | GCS bucket for Terraform state. |
| `reservation_affinity`, `static_node_count` | Consumption model and node count, see below. |

### Consumption options

The deployment file ships with **Option 1 (Specific Reservation)** uncommented.
This is the recommended model for production inference. To use another model,
comment out Option 1 and uncomment the one you want.

For Option 1, the reservation must be in `zone`, for `a3-edgegpu-8g` with
8 x `nvidia-h100-80gb`, and `static_node_count` must not exceed its free
capacity. For a shared reservation add `project: <OWNER_PROJECT>` under the
reservation name. See
[Using GCE Reservations](../../modules/compute/gke-node-pool/README.md#using-gce-reservations)
for details.

To scale the cluster, change `static_node_count` and make sure you have the
quota and reservation capacity for it. No other change is needed.

### DWS Flex Start (Options 2 and 3)

With Flex Start, `static_node_count` stays `$(null)` and the GPU pool scales
from zero as capacity is granted. Workloads must set
`nodeSelector: cloud.google.com/gke-flex-start: "true"`.

Submit and clean up a sample job:

```bash
kubectl apply -f examples/dws-sample-workloads/sample-job-flex.yaml
kubectl get jobs,pods
kubectl delete -f examples/dws-sample-workloads/sample-job-flex.yaml
```

With Option 3 (queued provisioning), jobs go through Kueue instead. They need
the label `kueue.x-k8s.io/queue-name: dws-local-queue` and the annotation
`provreq.kueue.x-k8s.io/maxRunDurationSeconds`:

```bash
kubectl apply -f examples/dws-sample-workloads/sample-job-flex-queue.yaml
kubectl get jobs,pods
kubectl delete -f examples/dws-sample-workloads/sample-job-flex-queue.yaml
```

## Deploy the cluster

```bash
cd ~/cluster-toolkit
make
./gcluster deploy examples/gke-a3-edgegpu/gke-a3-edgegpu-inference.yaml \
  -d examples/gke-a3-edgegpu/gke-a3-edgegpu-inference-deployment.yaml
```

Deployment takes about 20 minutes. When it finishes, connect and check the
cluster:

```bash
gcloud container clusters get-credentials <DEPLOYMENT_NAME> --region <REGION> --project <PROJECT_ID>

kubectl get nodes                       # GPU nodes, CPU pool, system pool all Ready
kubectl get gatewayclass                # gke-l7-regional-external-managed ... ACCEPTED True
kubectl get clusterqueue,localqueue     # a3-edge
```

## Serve a first model through the Inference Gateway

This is the path the blueprint is built for: a model server, an
`InferencePool` with its Endpoint Picker, and an `HTTPRoute` behind a GKE
Gateway. The example uses a small open model so no Hugging Face token is
needed.

1. Model server (1 GPU):

   ```bash
   cat <<'EOF' | kubectl apply -f -
   apiVersion: apps/v1
   kind: Deployment
   metadata: {name: vllm-qwen, labels: {app: vllm-qwen}}
   spec:
     replicas: 1
     selector: {matchLabels: {app: vllm-qwen}}
     template:
       metadata: {labels: {app: vllm-qwen}}
       spec:
         nodeSelector: {cloud.google.com/gke-accelerator: nvidia-h100-80gb}
         tolerations: [{key: nvidia.com/gpu, operator: Exists, effect: NoSchedule}]
         containers:
         - name: vllm
           image: vllm/vllm-openai:latest
           args: ["--model","Qwen/Qwen2.5-1.5B-Instruct","--served-model-name","qwen","--port","8000","--max-model-len","4096"]
           ports: [{containerPort: 8000}]
           resources: {limits: {nvidia.com/gpu: 1}}
           readinessProbe: {httpGet: {path: /health, port: 8000}, initialDelaySeconds: 60, periodSeconds: 10}
   EOF
   kubectl rollout status deployment/vllm-qwen --timeout=15m
   ```

2. InferencePool and Endpoint Picker (the EPP is scheduled on the CPU pool):

   ```bash
   helm install vllm-qwen-pool oci://registry.k8s.io/gateway-api-inference-extension/charts/inferencepool \
     --version v1.0.0 \
     --set inferencePool.modelServers.matchLabels.app=vllm-qwen \
     --set inferencePool.targetPortNumber=8000 \
     --set provider.name=gke
   ```

3. Gateway and route:

   ```bash
   cat <<'EOF' | kubectl apply -f -
   apiVersion: gateway.networking.k8s.io/v1
   kind: Gateway
   metadata: {name: inference-gateway}
   spec:
     gatewayClassName: gke-l7-regional-external-managed
     listeners: [{name: http, protocol: HTTP, port: 80}]
   ---
   apiVersion: gateway.networking.k8s.io/v1
   kind: HTTPRoute
   metadata: {name: qwen-route}
   spec:
     parentRefs: [{name: inference-gateway}]
     rules:
     - matches: [{path: {type: PathPrefix, value: /}}]
       backendRefs: [{group: inference.networking.k8s.io, kind: InferencePool, name: vllm-qwen-pool}]
   EOF
   kubectl get gateway inference-gateway -w   # wait for ADDRESS and PROGRAMMED=True (a few minutes)
   ```

4. Send a request:

   ```bash
   IP=$(kubectl get gateway inference-gateway -o jsonpath='{.status.addresses[0].value}')
   curl -s http://$IP/v1/chat/completions -H 'Content-Type: application/json' \
     -d '{"model":"qwen","messages":[{"role":"user","content":"Hello"}],"max_tokens":32}'
   ```

If the Gateway stays `Programmed=False` with a `NetworkServices API is not
enabled` message, enable `networkservices.googleapis.com` (see Prerequisites)
and wait a couple of minutes.

For production serving, replace the model server with your own deployment and
use `InferenceObjective` resources to set per-model priorities; see the
[GKE Inference Gateway documentation](https://cloud.google.com/kubernetes-engine/docs/concepts/about-gke-inference-gateway).

## Using Local SSD and GCS from pods

- **Local SSD (KV cache, scratch)**: `emptyDir` volumes on GPU nodes are backed
  by the node's Local SSDs. Mount an `emptyDir` in your model server and point
  the KV-cache offload or download directory at it.
- **GCS FUSE (model weights)**: grant the workload service account
  (`<DEPLOYMENT_NAME>-gke-wl-sa@<PROJECT_ID>.iam.gserviceaccount.com`) access to
  your bucket, then mount it with the CSI driver using the
  `workload-identity-k8s-sa` Kubernetes service account in the `default`
  namespace. See
  [Cloud Storage FUSE CSI driver](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/cloud-storage-fuse-csi-driver).

## Verify multi-node GPU networking (optional)

For multi-node serving, verify NCCL over GPUDirect-TCPX between two nodes:

```bash
kubectl apply -f examples/gke-a3-edgegpu/nccl-test.yaml
kubectl wait pod/nccl-test-host-1 pod/nccl-test-host-2 --for=condition=Ready --timeout=5m
kubectl exec -t nccl-test-host-1 -c nccl-test -- /bin/bash -c "cp /configs/allgather.sh /scripts/allgather.sh"
kubectl exec -t nccl-test-host-1 -c nccl-test -- /scripts/allgather.sh nccl-host-1 nccl-host-2
kubectl delete -f examples/gke-a3-edgegpu/nccl-test.yaml
```

Expected: `Out of bounds values : 0` and a bus bandwidth of roughly 25 GB/s
for message sizes of 32 MiB and above.

## Running jobs through Kueue

Batch jobs can be submitted through the `a3-edge` LocalQueue by adding the
label `kueue.x-k8s.io/queue-name: a3-edge`. The ClusterQueue quota equals the
total number of GPUs in the node pool.

## Clean up

Delete Gateway resources first so their load balancers are removed, then
destroy the deployment:

```bash
kubectl delete httproute --all
kubectl delete gateway --all
helm uninstall vllm-qwen-pool
./gcluster destroy <DEPLOYMENT_NAME> --auto-approve
```

## Notes

- The GPU node pool is zonal. Spot nodes (`spot: true`) can be reclaimed by
  Compute Engine at any time, so use Spot only for workloads that tolerate
  interruption.
