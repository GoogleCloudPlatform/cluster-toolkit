# GKE A5X Bare Metal cluster with Cluster Toolkit

This blueprint creates a GKE cluster with A5X Bare Metal nodes (`a5x-highgpu-4g-metal`: 4 × NVIDIA VR200 GPUs per node, NVLink domains of up to 18 nodes / 72 GPUs) using [Cluster Toolkit](https://docs.cloud.google.com/cluster-toolkit/docs/overview).

The blueprint creates:

* A VPC with Cloud NAT for the node network (`eth0`).
* Service accounts for the nodes and for workloads (Workload Identity).
* A GKE cluster (Dataplane V2, no release channel) with a system node pool.
* A workload policy for a 1x72 GPU topology, and one or more A5X node pools that use the A5X reservation, pinned to reservation sub-blocks.
* A5X nodes on the custom node image with the NVIDIA driver and IMEX preloaded, and a DaemonSet that loads them.
* The NVIDIA DRA driver (ComputeDomains for multi-node NVLink) with its pod quota, JobSet and Kueue.
* Cloud Storage buckets mounted through GCS FUSE, and the matching persistent volumes.
* Two job templates: `nvidia-smi` on each node, and a fio storage benchmark.

## Before you begin

1. A Google Cloud project with billing enabled, and these APIs enabled:

    ```bash
    gcloud services enable compute.googleapis.com container.googleapis.com \
      iam.googleapis.com storage.googleapis.com --project=PROJECT_ID
    ```

2. Permissions for the user running `gcluster` to create networks, GKE clusters, service accounts and their IAM bindings, and Cloud Storage buckets: for example `roles/owner`, or `roles/compute.admin`, `roles/container.admin`, `roles/iam.serviceAccountAdmin`, `roles/iam.serviceAccountUser`, `roles/resourcemanager.projectIamAdmin` and `roles/storage.admin`. Listing reservation sub-blocks needs `roles/compute.viewer` on the reservation's project.
3. `gcloud`, Terraform, `kubectl`, Go and `make`. [Cloud Shell](https://docs.cloud.google.com/shell/docs/launching-cloud-shell) has all of them; for another environment, see [install dependencies](https://docs.cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
4. A Cloud Storage bucket in your project for the A5X files (A5X_BUCKET), with upload access for your Google Cloud account team.
5. From your account team:
   * allowlisting for the A5X machine type, and an A5X reservation;
   * the files for creating the node image (Image Builder files or the prebuilt image archive), uploaded to A5X_BUCKET;
   * the GKE version and NVIDIA driver version to use with the node image.

## Get Cluster Toolkit

Clone the repository and build `gcluster`:

```bash
git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
cd cluster-toolkit
make
./gcluster --version
```

Run the remaining commands from the `cluster-toolkit` folder.

## Get the node image

A5X nodes use a custom COS node image with the NVIDIA driver and IMEX preloaded, created in your project in one of two ways. Set `custom_image_source` in the deployment file to match:

* `prebuilt`: create the image from the prebuilt GKE A5X image archive (`IMAGE_NAME.tar.gz`) in your A5X bucket:

    ```bash
    gcloud compute images create IMAGE_NAME \
      --project=PROJECT_ID \
      --source-uri=gs://A5X_BUCKET/IMAGE_NAME.tar.gz \
      --guest-os-features=UEFI_COMPATIBLE,IDPF \
      --architecture=ARM64 \
      --labels=gke-golden-image=non_prod
    ```

* `image-builder`: build the image with Image Builder, using the files and instructions from your account team.

Each kind needs its own driver DaemonSet; the blueprint automatically installs the matching one. Check the image:

```bash
gcloud compute images describe IMAGE_NAME --project=PROJECT_ID \
  --format='yaml(status,architecture,guestOsFeatures)'
```

* Expect `status: READY`, `architecture: ARM64`, and `UEFI_COMPATIBLE` and `IDPF` in `guestOsFeatures`.
* Keep `cos` in `IMAGE_NAME`. GKE uses the `cos` in the name to identify the image as COS. Without it, the GPU device plugin doesn't run and the nodes report no GPUs.

## Create the cluster

1. Create a Cloud Storage bucket for the Terraform state, separate from A5X_BUCKET:

    ```bash
    gcloud storage buckets create gs://BUCKET_NAME \
        --default-storage-class=STANDARD \
        --project=PROJECT_ID \
        --location=COMPUTE_REGION_TERRAFORM_STATE \
        --uniform-bucket-level-access
    gcloud storage buckets update gs://BUCKET_NAME --versioning
    ```

2. Fill in `gke-a5x-bm-deployment.yaml`:

    ```yaml
    terraform_backend_defaults:
      type: gcs
      configuration:
        bucket: BUCKET_NAME
    vars:
      project_id: PROJECT_ID
      deployment_name: DEPLOYMENT_NAME   # 6-30 characters; also the cluster name
      region: COMPUTE_REGION             # region of the reservation
      zone: ZONE                         # zone of the reservation
      static_node_count: 18              # nodes per node pool, 18 or fewer
      num_node_pools: 1                  # one node pool per sub-block
      authorized_cidr: IP_ADDRESS/32     # must include the machine running gcluster
      reservation: RESERVATION_NAME      # bare reservation name
      reservation_project_id: PROJECT_ID # owner project if the reservation is shared
      pin_to_subblocks: true             # gcluster picks free sub-blocks and saves them below
      reservation_subblocks: {}          # sub-block map; leave empty on the first deploy
      custom_image: IMAGE_NAME
      custom_image_source: IMAGE_SOURCE  # image-builder or prebuilt
      version_prefix: "GKE_VERSION"      # GKE version for the node image, from your account team
      nvidia_driver_version: "DRIVER_VERSION"  # image-builder: driver in the image; comment out for prebuilt
    ```

3. Deploy:

    ```bash
    gcloud auth application-default login
    ./gcluster deploy -d \
      examples/gke-a5x-bm/gke-a5x-bm-deployment.yaml \
      examples/gke-a5x-bm/gke-a5x-bm.yaml
    ```

    Confirm the sub-block proposal if asked, then select **(A)pply**. The output shows the credentials command and the generated job files: `DEPLOYMENT_NAME/primary/run-nvidia-smi-*.yaml` and `DEPLOYMENT_NAME/primary/storage-fio-job-*.yaml`.

To resize later, change `static_node_count` and run `./gcluster deploy` again. The node pool is resized in place.

### Sub-block pinning

A sub-block is one rack: 18 hosts, one NVLink domain. With `pin_to_subblocks: true`, each A5X node pool is pinned to its own sub-block (`num_node_pools` = number of sub-blocks). On the first deploy, `gcluster`:

1. lists the sub-blocks of your reservation;
2. proposes free ones (`HEALTHY` first, then `DEGRADED` with a warning);
3. asks before saving them to the deployment file (keeping a timestamped backup).

Later deploys reuse the saved map, so node pools don't move.

```text
module "a5x-pool": proposed sub-block map (1 new):
  reservation_subblocks:
    "01-0001": RESERVATION/reservationBlocks/RESERVATION-block-0001/reservationSubBlocks/RESERVATION-block-0001-subblock-0001  # new
Save these sub-blocks for module "a5x-pool" to examples/gke-a5x-bm/gke-a5x-bm-deployment.yaml? [y/n]
```

* Node pools are named `a5x-sb-BB-SSSS`, for example `a5x-sb-01-0001`.
* A sub-block is free only if none of its hosts are in use. `DEGRADED` means some hosts may be unhealthy, so the node pool may get fewer nodes, or nodes with broken NVLink.
* To move a node pool to another sub-block, delete its entry from `reservation_subblocks` and deploy again. The node pool is replaced.

## Advanced settings

Set these in `vars` in `gke-a5x-bm.yaml`, or override them in the deployment file.

* `custom_image_project` (default: `project_id`): the project that holds the node image.
* `custom_image_source` (default `image-builder`): how `custom_image` was made, which selects the driver DaemonSet. `image-builder` installs `nvidia-preloaded-driver-installer`; `prebuilt` installs `nvidia-driver-installer` (from `nvidia-prebuilt-driver-installer.yaml`), which finds the driver version in the image.
* `nvidia_driver_version` (`image-builder` only): the NVIDIA driver version built into `custom_image`. It must match the NVIDIA bundle the image was built from; the driver installer DaemonSet loads this version.
* `version_prefix`: the GKE version to use with the node image.
* `ignore_node_count_changes` (default `true`): keeps A5X node pool size changes made outside Terraform (for example with `gcloud` or an autoscaling controller). Changing `static_node_count` still resizes the node pool. Set `false` to reset the size to `static_node_count` on every deploy.
* Boot disk sizes: `system_node_pool_disk_size_gb` (200) and `a5x_node_pool_disk_size_gb` (400).
* Node auto-upgrade and auto-repair are off for the A5X node pool; the cluster isn't in a release channel. The custom image is built for one GKE version.

## Verify the cluster

1. Get credentials:

    ```bash
    gcloud container clusters get-credentials DEPLOYMENT_NAME --location=COMPUTE_REGION
    ```

2. Nodes are `Ready` with 4 GPUs, on the expected sub-block:

    ```bash
    kubectl get nodes -l cloud.google.com/gke-accelerator=nvidia-vr200 \
      -o custom-columns='NAME:.metadata.name,READY:.status.conditions[?(@.type=="Ready")].status,GPU:.status.allocatable.nvidia\.com/gpu,EPHEMERAL:.status.capacity.ephemeral-storage,POOL:.metadata.labels.cloud\.google\.com/gke-nodepool,SUBBLOCK:.metadata.labels.cloud\.google\.com/gce-topology-subblock'
    ```

    All nodes of a pinned node pool show the same `SUBBLOCK`. `EPHEMERAL` is about 46 TiB: the 4 local SSDs back ephemeral storage.

3. The driver installer loaded the driver on every A5X node. With `custom_image_source: image-builder`:

    ```bash
    kubectl get daemonset -n kube-system nvidia-preloaded-driver-installer
    kubectl logs -n kube-system -l k8s-app=nvidia-preloaded-driver-installer -c nvidia-driver-installer --tail=-1 --prefix \
      | grep "GPU drivers successfully installed"
    ```

    With `custom_image_source: prebuilt`:

    ```bash
    kubectl get daemonset -n kube-system nvidia-driver-installer
    kubectl logs -n kube-system -l k8s-app=nvidia-driver-installer -c nvidia-driver-installer --tail=-1 --prefix \
      | grep -E "Driver successfully moved and loaded|already installed"
    ```

4. Every node can join multi-node NVLink:

    ```bash
    kubectl get pods -n nvidia-dra-driver-gpu
    kubectl get resourceslices -o custom-columns='DRIVER:.spec.driver,NODE:.spec.nodeName' | grep compute-domain
    ```

    All `nvidia-dra-driver-gpu-kubelet-plugin-*` pods are `Running`, with one `compute-domain.nvidia.com` slice per A5X node.

5. `nvidia-smi` on every node (replace `SUFFIX`):

    ```bash
    kubectl create -f DEPLOYMENT_NAME/primary/run-nvidia-smi-SUFFIX.yaml
    kubectl wait --for=condition=complete job/run-nvidia-smi-SUFFIX --timeout=300s
    kubectl logs -l job-name=run-nvidia-smi-SUFFIX --tail=-1 --prefix
    kubectl delete job run-nvidia-smi-SUFFIX
    ```

    Each pod lists 4 GPUs with `0MiB / 286524MiB`.

6. CUDA test with `a5x-cuda-test.yaml` (one pod on one A5X node):

    ```bash
    kubectl apply -f examples/gke-a5x-bm/a5x-cuda-test.yaml
    kubectl wait --for=jsonpath='{.status.phase}'=Succeeded pod/a5x-cuda-test --timeout=300s
    kubectl logs a5x-cuda-test
    kubectl delete -f examples/gke-a5x-bm/a5x-cuda-test.yaml
    ```

    Ends with `Test PASSED`.

7. Storage benchmark (one pod on one A5X node):

    ```bash
    kubectl create -f DEPLOYMENT_NAME/primary/storage-fio-job-SUFFIX.yaml
    kubectl wait --for=condition=complete job/storage-fio-job-SUFFIX --timeout=1200s
    kubectl logs -l job-name=storage-fio-job-SUFFIX -c storage-fio-job-container --tail=-1 | grep -E "WRITE:|READ:|Permission"
    kubectl delete job storage-fio-job-SUFFIX
    ```

## Run NCCL tests

The tests use the NCCL test image `us-docker.pkg.dev/gce-ai-infra/gpudirect-gib/nccl-vr200-container:v08-2.32.6`, which has NCCL 2.32.6 and nccl-tests built in.

Run one test at a time; each uses all 4 GPUs of a node.

### Single node

`nccl-test-single-node.yaml` runs `all_reduce_perf` on the 4 GPUs of one node.

```bash
kubectl apply -f examples/gke-a5x-bm/nccl-test-single-node.yaml
kubectl wait --for=jsonpath='{.status.phase}'=Succeeded pod/nccl-test-single-node --timeout=1200s
kubectl logs nccl-test-single-node | grep -E "nccl-library|^\s+8589934592|Out of bounds|Avg bus"
kubectl delete -f examples/gke-a5x-bm/nccl-test-single-node.yaml
```

Expected: `nccl-library=23206` (NCCL 2.32.6) and `Out of bounds values : 0 OK`. Reference: about **1,020 GB/s** bus bandwidth at 8 GiB.

### Two nodes over multi-node NVLink (ComputeDomain)

`nccl-test-compute-domain.yaml` creates a ComputeDomain over 2 nodes and 2 test pods. Both nodes need a `compute-domain.nvidia.com` slice (verify step 4).

```bash
kubectl apply -f examples/gke-a5x-bm/nccl-test-compute-domain.yaml
kubectl get computedomain nccl-test-compute-domain -o yaml | sed -n '/^status:/,$p'
kubectl logs nccl-test-host-1 | tail -n 3
kubectl logs nccl-test-host-2 | tail -n 3
```

Wait until the ComputeDomain is `Ready` (both nodes, same `cliqueID`) and both pods print `ComputeDomain Template Booted!`. Then run all-reduce (or `all_gather_perf`):

```bash
kubectl exec nccl-test-host-1 -- bash -c '
set -eo pipefail
mpirun --allow-run-as-root \
  --hostfile /tmp/hostfile --bind-to none \
  --mca btl self,tcp --mca btl_tcp_if_include eth0 \
  --mca plm_rsh_no_tree_spawn 1 --mca btl_tcp_disable_family 6 \
  --mca plm_rsh_agent "ssh -q -p 222 -o LogLevel=ERROR -o StrictHostKeyChecking=no -o BatchMode=yes" \
  -x PATH="/var/nccl-tests/build:${PATH}" \
  -x LD_LIBRARY_PATH="/var/nccl/current/lib:/usr/local/cuda-13.5/lib64:/opt/rdma-core/build/lib:/usr/lib/aarch64-linux-gnu:/usr/local/nvidia/lib64" \
  -x UCX_NET_DEVICES=eth0 \
  -x NCCL_DEBUG=INFO -x NCCL_DEBUG_SUBSYS=INIT \
  -x NCCL_SOCKET_IFNAME=eth0 -x NCCL_IB_DISABLE=1 -x NCCL_MNNVL_ENABLE=1 \
  -x NCCL_NVLINK_UTIL_CENTRIC_SCHED_ENABLE=0 \
  -x HWLOC_COMPONENTS="-pci" \
  all_reduce_perf -b 1K -e 8G -f 2 -g 1 -w 5 -n 50'
kubectl delete -f examples/gke-a5x-bm/nccl-test-compute-domain.yaml
```

Check for `via P2P/MNNVL` and `Out of bounds values : 0 OK`. Reference (2 nodes, one rack): all-reduce about **1,470 GB/s**, all-gather about **1,080 / 1,090 GB/s** (out-of-place / in-place) at 8 GiB.

### Multiple nodes with JobSet and Kueue

`nccl-jobset-example.yaml` runs `all_gather_perf` as a JobSet submitted to the `a5x` Kueue queue. Kueue admits it only when all worker pods fit in one sub-block. It runs on 2 nodes by default. For another node count (up to 18), change the 4 values marked `# NUM_NODES` in the manifest to that count.

```bash
kubectl create -f examples/gke-a5x-bm/nccl-jobset-example.yaml
kubectl get workloads
kubectl get pods -l jobset.sigs.k8s.io/jobset-name=allgather
```

When the `allgather-worker-0-0-*` pod is `Completed`, check the results:

```bash
HEAD=$(kubectl get pods -l jobset.sigs.k8s.io/jobset-name=allgather,batch.kubernetes.io/job-completion-index=0 -o name)
kubectl logs "$HEAD" | grep -m1 "via P2P/MNNVL"
kubectl logs "$HEAD" | grep -E "^\s+8589934592|Out of bounds|Avg bus"
kubectl delete -f examples/gke-a5x-bm/nccl-jobset-example.yaml --ignore-not-found
```

Check for `via P2P/MNNVL` and `Out of bounds values : 0 OK`. Reference (2 nodes, one rack): all-gather about **1,080 / 1,090 GB/s** (out-of-place / in-place) at 8 GiB.

The manifest creates a ComputeDomain and the JobSet; the delete command removes both. Delete them before running the test again with a different node count.

## Known limitations

* Workloads can't span more than one rack: cross-rack (back-end) networking isn't enabled yet.
* Set `UCX_NET_DEVICES=eth0` and `NCCL_SOCKET_IFNAME=eth0` in your workloads so UCX and NCCL bind to `eth0`; the test manifests already do.
* GPU persistence mode is disabled on A5X nodes.

## Clean up

To avoid recurring charges for the resources, clean up the resources that are provisioned by Cluster Toolkit:

```bash
cd cluster-toolkit
./gcluster destroy DEPLOYMENT_NAME
```
