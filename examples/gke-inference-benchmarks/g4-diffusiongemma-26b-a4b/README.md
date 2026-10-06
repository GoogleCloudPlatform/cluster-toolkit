# DiffusionGemma 26B-A4B vLLM inference benchmark on a Spot G4 node pool

This benchmark provisions a GKE cluster with a single-zone **Spot** G4 node pool
(`g4-standard-48`, 1x NVIDIA RTX PRO 6000), serves
[`google/diffusiongemma-26B-A4B-it`](https://huggingface.co/google/diffusiongemma-26B-A4B-it)
with [vLLM](https://docs.vllm.ai/) and runs a `vllm bench serve` throughput and
latency benchmark against it, all from a single `gcluster deploy`.

The serving flags and benchmark parameters mirror the single-GPU DiffusionGemma
G4 recipe used by Google's internal inference benchmarking (uBench,
`g4/v0_30_x/diffusiongemma_26b_a4b_it_1gpu_bf16_isl1024_osl1024`), so the
numbers you get are comparable with Google's reference results below.

Everything the benchmark needs is in this directory:

| File | Purpose |
| --- | --- |
| [`blueprint.yaml`](blueprint.yaml) | VPC, service accounts, GKE cluster, Spot G4 node pool and the `kubectl-apply` step that installs the manifests. |
| [`deployment.yaml`](deployment.yaml) | The values you fill in: project, Terraform state bucket, region/zone, authorized CIDR. |
| [`manifests/vllm-serve.yaml`](manifests/vllm-serve.yaml) | StorageClass + PersistentVolumeClaim (Hugging Face cache), vLLM `Deployment` and `Service`. |
| [`manifests/vllm-bench.yaml`](manifests/vllm-bench.yaml) | `vllm bench serve` `Job` that waits for the server and prints the results. |
| [`manifests/hf-secret.yaml.tftpl`](manifests/hf-secret.yaml.tftpl) | `hf-secret` `Secret`, rendered from the `hf_token` variable (optional). |

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

## Deploy

1. Open [`deployment.yaml`](deployment.yaml) and set `project_id`, `bucket`,
   `authorized_cidr` (`<your-ip>/32`) and, if needed, `region`/`zone`. Leave
   `hf_token` empty.

1. Deploy from the repository root, passing the token on the command line:

   ```shell
   ./gcluster deploy -d examples/gke-inference-benchmarks/g4-diffusiongemma-26b-a4b/deployment.yaml \
     examples/gke-inference-benchmarks/g4-diffusiongemma-26b-a4b/blueprint.yaml --vars hf_token=$HF_TOKEN
   ```

   Type `a` at the prompt to apply. If you prefer not to pass the token through
   Terraform, omit `--vars hf_token=...` and create the Secret yourself once the
   cluster exists:

   ```shell
   kubectl create secret generic hf-secret --from-literal=hf_api_token=$HF_TOKEN
   ```

1. Connect to the cluster and check that the Spot G4 node is ready (replace
   `g4-diffusiongemma` with your `deployment_name` if you changed it):

   ```shell
   gcloud container clusters get-credentials g4-diffusiongemma --region us-central1 --project <project_id>
   kubectl get nodes -L cloud.google.com/gke-spot,cloud.google.com/gke-accelerator
   ```

1. Wait for the vLLM server. The first start downloads about 52 GB of weights
   to a Hyperdisk Balanced volume and loads them, which takes 15-25 minutes:

   ```shell
   kubectl rollout status deployment/diffusiongemma-vllm --timeout=40m
   kubectl logs deployment/diffusiongemma-vllm --tail=20
   ```

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
the raw result JSON.

To run a different workload, edit the `INPUT_LEN`, `OUTPUT_LEN`, `NUM_PROMPTS`
and `MAX_CONCURRENCY` environment variables in
[`manifests/vllm-bench.yaml`](manifests/vllm-bench.yaml), then re-create the
Job:

```shell
kubectl delete job diffusiongemma-vllm-bench
kubectl apply -f examples/gke-inference-benchmarks/g4-diffusiongemma-26b-a4b/manifests/vllm-bench.yaml
```

Keep `OUTPUT_LEN` a multiple of 256 (the diffusion canvas) and
`MAX_CONCURRENCY` at or below the server's `--max-num-seqs` (8), otherwise
requests queue inside the server and latency numbers stop being meaningful.

To deploy only the server (no Job), set `run_benchmark: false` in
[`deployment.yaml`](deployment.yaml).

### Reference results

Single run measured on 2026-10-06 in `us-central1-b` on a Spot
`g4-standard-48` node with vLLM `v0.30.0` (ISL 1024 / OSL 1024, 64 prompts,
concurrency 8). Spot pricing and preemptions do not affect per-request
performance; your numbers should be close to these on the same shape.

| Metric | Value |
| --- | --- |
| Successful requests | 64 / 64 |
| Benchmark duration | 202.4 s |
| Output token throughput | 323.9 tok/s |
| Total token throughput (input + output) | 647.7 tok/s |
| Time to first token (TTFT) mean / median / P99 | 6941.7 / 6507.7 / 10119.4 ms |
| Time per output token (TPOT) mean | 17.8 ms |
| Inter-token latency (ITL) median | 6149.9 ms |
| End-to-end latency (E2EL) median / P99 | 24938.5 / 28570.7 ms |

Server-side, the 48.5 GiB of weights loaded in 138 s from the cached volume
and the KV cache was sized at 30.19 GiB (143,374 tokens). The large ITL median
is expected: DiffusionGemma emits a whole 256-token canvas at once after
roughly 48 denoising steps, so tokens arrive in bursts rather than one by one.

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
  `cloud.google.com/gke-spot=true:NoSchedule` taint; both manifests tolerate it.
* When the node is preempted, GKE recreates it (`auto_repair: true`) and the
  `Deployment` reschedules the server. The Hugging Face cache lives on a
  `PersistentVolumeClaim`, so the restart only reloads the weights from disk
  (138 s in the reference run) instead of downloading them again.
* A benchmark run that is interrupted by a preemption fails and is retried by
  the Job (`backoffLimit: 3`) once the server is healthy again; the retried run
  waits for `/v1/models` just like the first one. Discard interrupted runs when
  comparing numbers.
* Spot capacity for G4 is zonal and not guaranteed. If the node pool stays at 0
  nodes, try another zone or set `spot: false`.

## Clean up

```shell
./gcluster destroy g4-diffusiongemma --auto-approve
```

The Hyperdisk volume is deleted with the cluster (`reclaimPolicy: Delete`). The
GCS bucket holding the Terraform state is not deleted.
