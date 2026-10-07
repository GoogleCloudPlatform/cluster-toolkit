# GKE Inference Benchmarks

This directory is a catalog of self-contained, reproducible LLM inference
benchmarks on GKE accelerators (GPUs and TPUs). Each benchmark is a single
directory that contains everything needed to run it: a Cluster Toolkit
blueprint, a deployment file and the Kubernetes manifests the blueprint applies.
One `gcluster deploy` from a checkout of this repository (`develop` branch)
provisions the cluster and accelerator node pool, serves the model and runs the
benchmark. No other tooling is required.

Every entry pins its serving image tag, serving flags and benchmark workload
(dataset, input/output lengths, number of prompts, concurrency), so results
obtained from the same entry are directly comparable with each other and with
the reference results recorded in its README.

## Catalog

| Benchmark | Accelerator | Model | Serving stack | Workload | Reference result |
| --- | --- | --- | --- | --- | --- |
| [g4-diffusiongemma-26b-a4b](g4-diffusiongemma-26b-a4b/README.md) | 1x `g4-standard-48` (1x NVIDIA RTX PRO 6000), Spot | `google/diffusiongemma-26B-A4B-it` (BF16) | vLLM `v0.30.0` | random dataset, ISL 1024 / OSL 1024, 64 prompts, concurrency 8 | 324.9 output tok/s, median TTFT 6.5 s ([details](g4-diffusiongemma-26b-a4b/README.md#reference-results)) |

TPU entries follow the same layout; see [Adding a benchmark](#adding-a-benchmark).

## Directory layout

```text
examples/gke-inference-benchmarks/
├── README.md                         # this catalog
└── <accelerator>-<model>/            # one self-contained benchmark
    ├── README.md                     # prerequisites, deploy, results, clean up
    ├── blueprint.yaml                # network, cluster, node pool, token Secret, kubectl-apply of manifests/
    ├── deployment.yaml               # the values you fill in (project, bucket, zone, CIDR, weights bucket)
    └── manifests/                    # Kubernetes manifests applied in three stages by the blueprint
        ├── stage-weights.yaml.tftpl  # Stage 1: standalone Job to stage model weights to Cloud Storage
        ├── <server>-serve.yaml.tftpl # Stage 2: model server (storage, wait-for-weights initContainer, Deployment, Service)
        └── <server>-bench.yaml.tftpl # Stage 3: benchmark Job
```

`<accelerator>` is the GKE machine family (for example `g4`, `a4`) or, for
TPUs, the TPU generation and slice topology (for example `tpu7x-2x2x1`,
`v6e-8`). `<model>` is the lower-cased model name without the organization
prefix. File names are identical across benchmarks, so the commands below work
for every entry.

## Running a benchmark

1. Install the [Cluster Toolkit prerequisites](../../README.md#quickstart) and
   build the binary:

   ```shell
   make
   ```

1. Make sure the project has quota for the accelerator in the zone you intend
   to use. Benchmarks that default to `spot: true` need the *preemptible*
   (Spot) quota of that accelerator. Gated models additionally require that
   you accept the model license on Hugging Face and have an access token.

1. Open `examples/gke-inference-benchmarks/<benchmark>/deployment.yaml` and
   fill in `project_id`, the Terraform state `bucket`, `authorized_cidr` (the
   public IP of the machine you deploy from, as `<ip>/32`) and, if needed,
   `region`/`zone`. The remaining variables have working defaults. Do not put
   secrets in this file.

1. Deploy. The blueprint and deployment paths are the only parts of the
   command that change between benchmarks (`MODEL_BUCKET` is an existing
   Cloud Storage bucket in which the model weights are kept, see below):

   ```shell
   ./gcluster deploy -d examples/gke-inference-benchmarks/<benchmark>/deployment.yaml \
     examples/gke-inference-benchmarks/<benchmark>/blueprint.yaml \
     --vars hf_token=$HF_TOKEN,model_bucket=$MODEL_BUCKET
   ```

   Type `a` at the prompt to apply. `gcluster` returns once the cluster, node
   pool and manifests are in place; the model server keeps loading weights in
   the background and the benchmark Job starts as soon as the server is
   healthy. A token passed with `--vars` ends up in plaintext in the deployment
   folder's `terraform.tfvars` and in the Terraform state; each benchmark's
   README explains how to supply it with `kubectl patch secret` instead.

1. Follow the benchmark's `README.md` to watch the rollout, read the results
   (`kubectl logs job/<benchmark job>`) and change the workload parameters
   (input/output length, number of prompts, concurrency).

1. Clean up. The Terraform state bucket and the weights bucket are not
   deleted:

   ```shell
   ./gcluster destroy <deployment_name> --auto-approve
   ```

## Adding a benchmark

New entries are welcome. Keep them uniform so that users can run any benchmark
with the same commands:

* **Self-contained.** Put everything in
  `examples/gke-inference-benchmarks/<accelerator>-<model>/` and reference
  manifests from the blueprint with `$(ghpc_stage("manifests/<file>"))`.
  `ghpc_stage` resolves paths relative to the blueprint, so the directory can
  be copied or deployed from any working directory.
* **Same file names.** `blueprint.yaml`, `deployment.yaml`, `manifests/`,
  `README.md`.
* **A deployment file that renders without edits.** Leave `project_id`,
  `bucket` and `authorized_cidr` empty, but give every other variable a working
  default (use the region/zone in which you measured the reference results).
  The repository CI renders each `blueprint.yaml` + `deployment.yaml` pair with
  `gcluster create` and `terraform validate`.
* **Secrets through the `kubernetes-secret` module.** Accept tokens as a
  blueprint variable with an empty default and create the Secret with
  [`modules/security/kubernetes-secret`](../../modules/security/kubernetes-secret/README.md)
  (`cluster_id: null` when in the same deployment group as `gke-cluster`),
  which marks the value sensitive. Make the manifests `.tftpl` templates that
  take the Secret name from the module output
  (`template_vars: { hf_secret_name: $(hf-secret.secret_name) }`), so that
  Terraform creates the Secret before the pods; never mark the `secretKeyRef`
  optional. Document that a `--vars` token is stored in plaintext in
  `terraform.tfvars` and the state bucket, and give the `kubectl patch secret`
  alternative. Never commit a token.
* **Weights from Cloud Storage (three-stage sequence).** Take `model_bucket`
  (empty default) and `model_path` variables and pass them to the staging Job
  and server manifests as template variables. When `model_bucket` is set:
  1. **Stage 1 (`manifests/stage-weights.yaml.tftpl`):** A standalone
     single-pod `Job` (`parallelism: 1`, `completions: 1`) checks
     `gs://<bucket>/<path>` and, if the snapshot is missing, stages it from the
     model hub using a rolling per-file download $\to$ concurrent multipart
     upload $\to$ delete pipeline (`config.json` uploaded last). Running weight
     staging in a single-pod Job rather than inside the server pod's
     `initContainer` prevents multiple server pods in a distributed deployment
     from concurrently downloading and overwriting the same Cloud Storage
     objects, and bounds local scratch disk usage to ~15 GiB even for multi-TB
     models.
  2. **Stage 2 (`manifests/<server>-serve.yaml.tftpl`):** A lightweight
     `wait-for-weights` `initContainer` waits until `config.json` and
     `*.safetensors` exist in `gs://<bucket>/<path>`, then the server streams
     the weights with the
     [Run:ai Model Streamer](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/run-ai-model-streamer)
     (`vllm serve gs://<bucket>/<path> --load-format=runai_streamer`, plus
     `--served-model-name`).
  3. **Stage 3 (`manifests/<server>-bench.yaml.tftpl`):** Waits for the server
     health endpoint and runs the benchmark.
  Run the staging Job and server pods as the `workload-identity-k8s-sa` service
  account and give the workload service account `storage.objectAdmin` and
  `storage.bucketViewer`. Keep a disk fallback for an empty `model_bucket`.
* **Pin and explain every parameter.** Pin the serving image tag and spell out
  the serving flags (parallelism, attention backend, memory utilization, max
  sequences, max model length) and the benchmark parameters (dataset, ISL/OSL,
  number of prompts, concurrency) in the manifests, and explain in the README
  why each non-default value was chosen, so that others can reproduce the
  numbers and compare like with like.
* **Do not block `gcluster deploy` on model loading.** Use
  `wait_for_rollout: false` for the server and make the benchmark Job wait for
  the server's health endpoint itself.
* **A benchmark Job that cannot hang or report partial numbers.** Give the Job
  an `activeDeadlineSeconds` that covers the server's start-up budget plus a
  few runs, check the result JSON (`completed` equals the number of prompts)
  and exit non-zero otherwise so that `backoffLimit` retries the run. Run the
  Job on the system node pool (no accelerator node selector or Spot
  toleration), so that it survives a preemption of the accelerator node.
* **Spot by default where it makes sense.** Use a single-zone node pool, a
  `spot` variable that defaults to `true`, tolerate the
  `cloud.google.com/gke-spot=true:NoSchedule` taint in the server manifest and
  keep the weights in Cloud Storage or on a PersistentVolumeClaim so that a
  preempted node recovers quickly. Explain the preemption behaviour in the
  README.
* **TPU entries.** Start from [`examples/gke-tpu-7x`](../gke-tpu-7x) or
  [`examples/gke-tpu-v6e`](../gke-tpu-v6e): the node pool takes `machine_type`,
  `num_slices`, `tpu_topology` and `spot`, and TPU 7x additionally needs the
  `resource-policy` module. Workloads request `google.com/tpu` and select nodes
  with `cloud.google.com/gke-tpu-accelerator` and
  `cloud.google.com/gke-tpu-topology`. Use a TPU-capable serving image (for
  example vLLM TPU or JetStream) and name the directory after the slice, for
  example `tpu7x-2x2x1-<model>` or `v6e-8-<model>`.
* **README sections.** Prerequisites, Deploy, Benchmark results (including how
  to change the workload), Try the endpoint, What the benchmark does and why,
  Spot behaviour (if applicable), Reference results (date, zone, consumption
  model, number of runs) and Clean up.
* **Wire it into CI and this catalog.** Add a `run_test` line for the new
  `blueprint.yaml`/`deployment.yaml` pair to
  [`tools/validate_configs/validate_configs.sh`](../../tools/validate_configs/validate_configs.sh)
  (the directory itself is excluded from the generic blueprint scan because it
  also contains Kubernetes manifests), add a row to the [Catalog](#catalog)
  table above, and run `pre-commit run --all-files` before opening a pull
  request.
