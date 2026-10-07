# DiffusionGemma 26B-A4B vLLM inference benchmark on a Spot G4 node pool

> **Disclaimer.** This is an experimental, community-supported example, not an
> officially supported Google product. The blueprint, deployment file and
> manifests in this directory were generated with the help of an AI coding
> agent and then reviewed and run end to end by a human. Review them before
> using them for anything beyond benchmarking, and treat the reference results
> as a single measured run on Spot capacity, not a performance guarantee.

This benchmark provisions a GKE cluster with a single-zone **Spot** G4 node pool
(`g4-standard-48`, 1x NVIDIA RTX PRO 6000), serves
[`google/diffusiongemma-26B-A4B-it`](https://huggingface.co/google/diffusiongemma-26B-A4B-it)
with [vLLM](https://docs.vllm.ai/) and runs a `vllm bench serve` throughput and
latency benchmark against it, all from a single `gcluster deploy`. The model
weights are streamed from a Cloud Storage bucket of your choice with the
[Run:ai Model Streamer](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/run-ai-model-streamer);
the first deployment copies them there from Hugging Face (see
[Model weights](#model-weights-cloud-storage-and-the-runai-model-streamer)).

The vLLM image tag, serving flags and benchmark parameters (random dataset,
1024 input / 1024 output tokens, 64 prompts, concurrency 8) are pinned in the
manifests, so the numbers you get are directly comparable with the reference
results below and with other runs of this example.

Everything the benchmark needs is in this directory:

| File | Purpose |
| --- | --- |
| [`blueprint.yaml`](blueprint.yaml) | VPC, service accounts, GKE cluster (Dataplane V2; GCS FUSE, Filestore and Managed Lustre CSI drivers enabled), Spot G4 node pool, the `hf-secret` Kubernetes `Secret` (via the [`kubernetes-secret`](../../../modules/security/kubernetes-secret/README.md) module) and the `kubectl-apply` step that installs the manifests in three stages. |
| [`deployment.yaml`](deployment.yaml) | The values you fill in: project, Terraform state bucket, region/zone, authorized CIDR, model weights bucket. |
| [`manifests/stage-weights.yaml.tftpl`](manifests/stage-weights.yaml.tftpl) | **Stage 1:** Standalone `diffusiongemma-stage-weights` `Job` (enabled when `model_bucket` is set) that checks `gs://<model_bucket>/<model_path>` and stages the Hugging Face snapshot using a rolling per-file download $\to$ concurrent multipart upload $\to$ delete pipeline. |
| [`manifests/vllm-serve.yaml.tftpl`](manifests/vllm-serve.yaml.tftpl) | **Stage 2:** StorageClass + PersistentVolumeClaim, vLLM `Deployment` (`wait-for-weights` initContainer + server) and `Service`; rendered with `model_bucket`/`model_path`. |
| [`manifests/vllm-bench.yaml.tftpl`](manifests/vllm-bench.yaml.tftpl) | **Stage 3:** `vllm bench serve` `Job` that waits for the server, prints the results and fails if any request failed. |

The three manifests are Terraform templates. Their `${hf_secret_name}`
expression is the name of the Secret created by the module, which makes
Terraform create the Secret before the pods that mount it; the staging Job and
server manifests additionally take `${model_bucket}` and `${model_path}`.

## Prerequisites

1. **Model access.** Accept the license of
   [`google/diffusiongemma-26B-A4B-it`](https://huggingface.co/google/diffusiongemma-26B-A4B-it)
   on Hugging Face and create a
   [read access token](https://huggingface.co/settings/tokens). Export it as
   `HF_TOKEN` in the shell you deploy from.

1. **Spot G4 quota.** The node pool uses Spot VMs by default, which consume
   the *preemptible* quota: **Preemptible NVIDIA RTX PRO 6000 GPUs** (at least
   1) and preemptible CPUs for the `g4-standard-48` shape in the chosen region.
   Set `spot: false` in [`deployment.yaml`](deployment.yaml) to use on-demand
   capacity and quota instead.

1. **Zone with G4 capacity.** Spot capacity is zonal. Pick a zone that offers
   G4 in the
   [GPU regions and zones](https://cloud.google.com/compute/docs/gpus/gpu-regions-zones)
   table and set `region`/`zone` accordingly. The reference results below were
   measured in `us-central1-b`.

1. **Cluster Toolkit.** Install the
   [prerequisites](../../../README.md#quickstart), clone this repository and
   build the binary with `make`. You also need a GCS bucket for Terraform
   state and the public IP of the machine you deploy from (for example
   `curl -s ifconfig.me`).

1. **Bucket for the model weights.** An existing Cloud Storage bucket, ideally
   in the cluster's region, in which the Hugging Face snapshot is kept under
   `model_path` (default `models/diffusiongemma-26B-A4B-it`, about 49 GiB).
   Export its name (without `gs://`) as `MODEL_BUCKET`. A bucket in the
   deployment project needs no extra IAM setup; for other cases see
   [Model weights](#model-weights-cloud-storage-and-the-runai-model-streamer).
   Leave `model_bucket` empty to skip Cloud Storage and download the weights
   to a Hyperdisk volume instead.

## Deploy

1. Open [`deployment.yaml`](deployment.yaml) and set `project_id`, `bucket`,
   `authorized_cidr` (`<your-ip>/32`) and, if needed, `region`/`zone` and
   `model_path`. Leave `hf_token` empty; `model_bucket` can be set here or on
   the command line.

1. Deploy from the repository root, passing the token and the bucket on the
   command line:

   ```shell
   ./gcluster deploy -d examples/gke-inference-benchmarks/g4-diffusiongemma-26b-a4b/deployment.yaml \
     examples/gke-inference-benchmarks/g4-diffusiongemma-26b-a4b/blueprint.yaml \
     --vars hf_token=$HF_TOKEN,model_bucket=$MODEL_BUCKET
   ```

   Type `a` at the prompt to apply. The token is stored in the `hf-secret`
   Kubernetes Secret by the blueprint's `kubernetes-secret` module, which
   marks the value sensitive so that Terraform never prints it.

   > **Note:** a token passed with `--vars` is still written in plaintext to
   > `terraform.tfvars` in the deployment folder and to the Terraform state in
   > your state bucket. If that is not acceptable, omit `hf_token=...`: the
   > Secret is then created with an empty token, the staging Job (or server pod)
   > fails the gated download, and the benchmark Job keeps waiting for the
   > server. Once the cluster exists, patch the Secret and restart the Jobs/Deployment
   > so that they pick up the token:
   >
   > ```shell
   > kubectl patch secret hf-secret --type merge -p "{\"stringData\":{\"hf_api_token\":\"$HF_TOKEN\"}}"
   > kubectl delete pod -l app=diffusiongemma-stage-weights
   > kubectl rollout restart deployment/diffusiongemma-vllm
   > kubectl delete pod -l app=diffusiongemma-vllm-bench
   > ```
   >
   > A later `gcluster deploy` of the same deployment resets the Secret to the
   > `hf_token` value known to Terraform, so repeat the patch after it.

1. Connect to the cluster and check that the Spot G4 node is ready (replace
   `g4-diffusiongemma` with your `deployment_name` if you changed it):

   ```shell
   gcloud container clusters get-credentials g4-diffusiongemma --region us-central1 --project <project_id>
   kubectl get nodes -L cloud.google.com/gke-spot,cloud.google.com/gke-accelerator
   ```

1. Follow the three-stage benchmark sequence:
   - **Stage 1 (`job/diffusiongemma-stage-weights`):** When `model_bucket` is
     set, a single-pod Job on the system node pool checks
     `gs://<model_bucket>/<model_path>` and stages the Hugging Face snapshot if
     not already present (~4–5 minutes on the first cold run; instant on
     subsequent runs).
   - **Stage 2 (`deployment/diffusiongemma-vllm`):** Its `wait-for-weights`
     initContainer waits for Stage 1 to complete (`config.json` present in
     Cloud Storage), then `vllm` streams the weights into GPU memory (~44 s).
     With `model_bucket` empty, `vllm` downloads about 52 GB of weights to the
     Hyperdisk volume and loads them (15–25 minutes on first start).
   - **Stage 3 (`job/diffusiongemma-vllm-bench`):** Waits for `/v1/models` on
     the server and runs `vllm bench serve`.

   ```shell
   kubectl logs -f job/diffusiongemma-stage-weights
   kubectl rollout status deployment/diffusiongemma-vllm --timeout=40m
   kubectl logs deployment/diffusiongemma-vllm -c wait-for-weights
   kubectl logs deployment/diffusiongemma-vllm --tail=20
   ```

## Model weights: Cloud Storage and the Run:ai Model Streamer

With `model_bucket` set, the deployment follows the GKE guide
[Accelerate model loading with Run:ai Model Streamer](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/run-ai-model-streamer)
using a three-stage workflow designed to scale cleanly to both multi-pod
distributed deployments and multi-TB models:

* **Stage 1 (`job/diffusiongemma-stage-weights`):** A single-pod `Job`
  (`parallelism: 1`, `completions: 1`) on the on-demand system node pool lists
  `gs://<model_bucket>/<model_path>`. If it finds `config.json` and at least one
  `*.safetensors` it exits at once. Otherwise it stages the Hugging Face
  snapshot using a rolling per-file pipeline (`hf_hub_download` $\to$
  `transfer_manager.upload_chunks_concurrently` $\to$ immediate local delete,
  with at most 3 files in flight). Extracting weight staging into a separate
  single-pod Job instead of running it inside the server pod's `initContainer`
  prevents multiple server pods in a distributed deployment (`JobSet`,
  `LeaderWorkerSet`, or multi-replica `Deployment`) from concurrently
  downloading and overwriting the same Cloud Storage objects, and keeps peak
  local scratch usage under ~15 GiB even for multi-TB models. `config.json` is
  uploaded last as an atomic completion marker.
* **Stage 2 (`deployment/diffusiongemma-vllm`):** The server pod's lightweight
  `wait-for-weights` initContainer polls `gs://<model_bucket>/<model_path>`
  until `config.json` and `*.safetensors` are present, then the server runs
  `vllm serve gs://<model_bucket>/<model_path> --load-format=runai_streamer --model-loader-extra-config={"distributed":true}`
  (plus `--served-model-name google/diffusiongemma-26B-A4B-it`, so the
  benchmark Job and your clients keep using the Hugging Face model name). vLLM
  downloads only the small configuration and tokenizer files and streams the
  safetensors from Cloud Storage straight into GPU memory, in parallel; look
  for `Loading safetensors using Runai Model Streamer: 100% Completed` in the
  server log.
* Authentication is Workload Identity Federation for GKE: the pods run as the
  `workload-identity-k8s-sa` Kubernetes service account, which the blueprint
  binds to the `<deployment_name>-gke-wl-sa` Google service account with
  `roles/storage.objectAdmin` and `roles/storage.bucketViewer` on the project
  (the Run:ai GCS backend reads bucket metadata, which object roles alone do
  not allow). For a bucket in **another project**, grant both roles on the
  bucket to that service account before deploying:

  ```shell
  gcloud storage buckets add-iam-policy-binding gs://$MODEL_BUCKET \
    --member=serviceAccount:<deployment_name>-gke-wl-sa@<project_id>.iam.gserviceaccount.com \
    --role=roles/storage.objectAdmin
  gcloud storage buckets add-iam-policy-binding gs://$MODEL_BUCKET \
    --member=serviceAccount:<deployment_name>-gke-wl-sa@<project_id>.iam.gserviceaccount.com \
    --role=roles/storage.bucketViewer
  ```

* Weights stay in the bucket across deployments and clusters: the next
  `gcluster deploy`, and every restart after a Spot preemption, skips the
  download and only streams. To speed up repeated loads in one zone further,
  the GKE guide above describes enabling Cloud Storage Rapid Cache for the
  bucket in the cluster's zone
  (`gcloud storage buckets anywhere-caches create gs://$MODEL_BUCKET <zone>`,
  billed separately).
* Leave `model_bucket` empty to use the disk path instead: vLLM downloads the
  weights from Hugging Face into the Hyperdisk volume on first start and
  reloads them from disk afterwards. Both paths serve the model with the same
  flags, so benchmark numbers are comparable; only start-up time differs.

## Benchmark results

The `diffusiongemma-vllm-bench` Job polls `/v1/models` until the server is up,
then runs `vllm bench serve` with the random dataset, 1024 input / 1024 output
tokens, 64 prompts, 3 warm-up requests and a maximum concurrency of 8:

```shell
kubectl wait --for=condition=complete job/diffusiongemma-vllm-bench --timeout=60m
kubectl logs job/diffusiongemma-vllm-bench
```

The log contains the usual `vllm bench serve` summary (request throughput,
output and total token throughput, TTFT/TPOT/ITL/E2EL percentiles) followed by
the raw result JSON. The Job runs on the on-demand system node pool, not on the
Spot GPU node, so a preemption does not kill the client; a run in which not all
requests completed exits non-zero and is retried (`backoffLimit: 3`), and the
Job gives up after 90 minutes (`activeDeadlineSeconds: 5400`), which covers the
server's 40-minute start-up budget plus several runs.

To run a different workload, edit the `INPUT_LEN`, `OUTPUT_LEN`, `NUM_PROMPTS`
and `MAX_CONCURRENCY` environment variables in
[`manifests/vllm-bench.yaml.tftpl`](manifests/vllm-bench.yaml.tftpl), then
re-create the Job. The manifest is a template with a single expression, so
`sed` is enough to render it (a changed Job cannot be re-applied through
`gcluster deploy` because a Job's pod template is immutable):

```shell
kubectl delete job diffusiongemma-vllm-bench
sed 's/\${hf_secret_name}/hf-secret/' \
  examples/gke-inference-benchmarks/g4-diffusiongemma-26b-a4b/manifests/vllm-bench.yaml.tftpl | kubectl apply -f -
```

Keep `OUTPUT_LEN` a multiple of 256 (the diffusion canvas) and
`MAX_CONCURRENCY` at or below the server's `--max-num-seqs` (8), otherwise
requests queue inside the server and latency numbers stop being meaningful.

To deploy only the server (no Job), set `run_benchmark: false` in
[`deployment.yaml`](deployment.yaml).

### Reference results

Single runs measured in `us-central1-b` on a Spot `g4-standard-48` node with
vLLM `v0.30.0` (ISL 1024 / OSL 1024, 64 prompts, concurrency 8), both with
Cloud Storage + Run:ai Model Streamer (`model_bucket` set, 2026-10-07) and with
the Hyperdisk volume fallback (`model_bucket` empty, 2026-10-06). Spot pricing
and preemptions do not affect per-request performance; your numbers should be
close to these on the same shape.

| Metric | Cloud Storage + Run:ai Streamer (`model_bucket` set) | Hyperdisk volume (`model_bucket` empty) |
| --- | --- | --- |
| First-time weight staging (`stage-weights`) | 154 s HF download + 116 s GCS upload (48.1 GiB, 21 files) | — (downloaded by vLLM on first start) |
| Server weight load time (48.5 GiB, 1,047 tensors) | **44.3 s** (39 s streaming at 26.3 tensors/s) | 138.2 s |
| KV cache allocation | 30.17 GiB (143,281 tokens) | 30.19 GiB (143,374 tokens) |
| Successful requests | 64 / 64 | 64 / 64 |
| Benchmark duration | 201.7 s | 202.4 s |
| Output token throughput | **324.9 tok/s** | 323.9 tok/s |
| Total token throughput (input + output) | **649.9 tok/s** | 647.7 tok/s |
| Time to first token (TTFT) mean / median / P99 | 6490.3 / 6501.8 / 6579.3 ms | 6941.7 / 6507.7 / 10119.4 ms |
| Time per output token (TPOT) mean | 17.8 ms | 17.8 ms |
| Inter-token latency (ITL) median | 6138.3 ms | 6149.9 ms |
| End-to-end latency (E2EL) median / P99 | 24948.2 / 25976.8 ms | 24938.5 / 28570.7 ms |

The large ITL median is expected: DiffusionGemma emits a whole 256-token canvas
at once after roughly 48 denoising steps (5.38 committed tokens per step), so
tokens arrive in bursts rather than one by one. Streaming from Cloud Storage
with the Run:ai Model Streamer cut the server's weight loading time by more
than 3x (44.3 s vs. 138.2 s) while producing identical serving throughput.

## Try the endpoint

```shell
kubectl port-forward svc/diffusiongemma-vllm 8000:8000
```

```shell
curl -s http://localhost:8000/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "google/diffusiongemma-26B-A4B-it",
    "messages": [{"role": "user", "content": "Explain Spot VMs in two sentences."}],
    "max_tokens": 256
  }'
```

## What the benchmark does, and why

DiffusionGemma is an encoder-decoder *block-diffusion* model: instead of
decoding one token per step, each step denoises a 256-token canvas. vLLM
(`DiffusionGemmaModelForBlockDiffusionConfig`, supported since `v0.24.0`;
pinned here to `v0.30.0`) enforces constraints that make the server flags
deliberately different from a typical dense-model deployment:

| Flag | Value | Reason |
| --- | --- | --- |
| `--attention-backend` | `TRITON_ATTN` | vLLM rejects FlashInfer for the model's mixed causal/bidirectional attention, and FlashAttention lacks the required head size. |
| `--tensor-parallel-size` | `1` | Tensor parallelism > 1 is not supported for this model; the BF16 checkpoint (about 52 GB) fits on one 96 GB RTX PRO 6000. |
| `--max-num-seqs` | `8` | The sampler materializes a `[num_seqs, canvas, vocab]` fp32 tensor per step; more concurrent sequences run out of memory. |
| `--gpu-memory-utilization` | `0.85` | Leaves headroom for the sampler and Triton workspace next to the KV cache. |
| `--max-model-len` | `4096` | Covers ISL 1024 + OSL 1024 with margin while keeping the KV cache small. |
| `--reasoning-parser`, `--tool-call-parser` | `gemma4` | Parses the model's thinking and tool-call blocks in chat completions. |

The benchmark Job uses the same image and `--max-concurrency 8` so that the
client never exceeds the server's sequence budget.

## Spot behaviour

* Spot nodes are labelled `cloud.google.com/gke-spot=true` and may carry a
  `cloud.google.com/gke-spot=true:NoSchedule` taint; the server manifest
  tolerates it. The benchmark Job has no toleration or node selector on
  purpose, so it runs on the on-demand system node pool and is not evicted
  with the GPU node.
* When the node is preempted, GKE recreates it (`auto_repair: true`) and the
  `Deployment` reschedules the server. With `model_bucket` set the restart
  streams the weights from Cloud Storage again (the `stage-weights`
  initContainer finds them and exits); with it empty the Hugging Face cache on
  the `PersistentVolumeClaim` is reused and the weights are reloaded from disk
  (138 s in the reference run). Neither path downloads from Hugging Face again.
* A benchmark run that is interrupted by a preemption ends with failed
  requests; the Job detects this (`completed` < `NUM_PROMPTS` in the result
  JSON), exits non-zero and is retried (`backoffLimit: 3`) once the server is
  healthy again; the retried run waits for `/v1/models` just like the first
  one. Discard interrupted runs when comparing numbers.
* Spot capacity for G4 is zonal and not guaranteed. If the node pool stays at 0
  nodes, try another zone or set `spot: false`.

## Clean up

```shell
./gcluster destroy g4-diffusiongemma --auto-approve
```

The Hyperdisk volume is deleted with the cluster (`reclaimPolicy: Delete`). The
GCS buckets holding the Terraform state and the model weights are not deleted;
remove the weights with
`gcloud storage rm -r gs://$MODEL_BUCKET/models/diffusiongemma-26B-A4B-it` if
you no longer need them.
