# GKE Consumption Options Blueprints

This directory contains Google Kubernetes Engine (GKE) blueprints configured for **Dynamic Workload Scheduler (DWS) Flex Start**, **Queued Provisioning**, and **Compact Placement** across AI Hypercomputer GPUs (`a3-ultragpu-8g`), TPUs (`tpu-v6e`, `tpu-7x`), and Cloud RDMA HPC VMs (`h4d-standard-192`).

## Available Blueprints

| Consumption Option | Hardware / Machine Type | Blueprint Path | Companion Deployment File | Guide |
| :--- | :--- | :--- | :--- | :--- |
| **DWS Flex Start** | **A3 Ultra (`a3-ultragpu-8g`)** | [`dws-flex-start/gke-a3-ultragpu.yaml`](./dws-flex-start/gke-a3-ultragpu.yaml) | [`dws-flex-start/gke-a3-ultragpu-deployment.yaml`](./dws-flex-start/gke-a3-ultragpu-deployment.yaml) | [`dws-flex-start/README.md`](./dws-flex-start/README.md) |
| **DWS Flex Start** | **TPU 7x (`tpu7x-standard-4t`)** | [`dws-flex-start/gke-tpu-7x/gke-tpu-7x.yaml`](./dws-flex-start/gke-tpu-7x/gke-tpu-7x.yaml) | [`dws-flex-start/gke-tpu-7x/gke-tpu-7x-deployment.yaml`](./dws-flex-start/gke-tpu-7x/gke-tpu-7x-deployment.yaml) | [`dws-flex-start/gke-tpu-7x/README.md`](./dws-flex-start/gke-tpu-7x/README.md) |
| **DWS Flex Start** | **TPU v6e (`ct6e-standard-4t`)** | [`dws-flex-start/gke-tpu-v6e/gke-tpu-v6e.yaml`](./dws-flex-start/gke-tpu-v6e/gke-tpu-v6e.yaml) | [`dws-flex-start/gke-tpu-v6e/gke-tpu-v6e-deployment.yaml`](./dws-flex-start/gke-tpu-v6e/gke-tpu-v6e-deployment.yaml) | [`dws-flex-start/gke-tpu-v6e/README.md`](./dws-flex-start/gke-tpu-v6e/README.md) |
| **DWS Flex Start + Queued Provisioning** | **A3 Ultra (`a3-ultragpu-8g`)** | [`dws-flex-start-queued-provisioning/gke-a3-ultragpu.yaml`](./dws-flex-start-queued-provisioning/gke-a3-ultragpu.yaml) | [`dws-flex-start-queued-provisioning/gke-a3-ultragpu-deployment.yaml`](./dws-flex-start-queued-provisioning/gke-a3-ultragpu-deployment.yaml) | [`dws-flex-start-queued-provisioning/README.md`](./dws-flex-start-queued-provisioning/README.md) |
| **DWS Flex Start + Queued Provisioning** | **TPU 7x (`tpu7x-standard-4t`)** | [`dws-flex-start-queued-provisioning/gke-tpu-7x/gke-tpu-7x.yaml`](./dws-flex-start-queued-provisioning/gke-tpu-7x/gke-tpu-7x.yaml) | [`dws-flex-start-queued-provisioning/gke-tpu-7x/gke-tpu-7x-deployment.yaml`](./dws-flex-start-queued-provisioning/gke-tpu-7x/gke-tpu-7x-deployment.yaml) | [`dws-flex-start-queued-provisioning/gke-tpu-7x/README.md`](./dws-flex-start-queued-provisioning/gke-tpu-7x/README.md) |
| **DWS Flex Start + Queued Provisioning** | **TPU v6e (`ct6e-standard-4t`)** | [`dws-flex-start-queued-provisioning/gke-tpu-v6e/gke-tpu-v6e.yaml`](./dws-flex-start-queued-provisioning/gke-tpu-v6e/gke-tpu-v6e.yaml) | [`dws-flex-start-queued-provisioning/gke-tpu-v6e/gke-tpu-v6e-deployment.yaml`](./dws-flex-start-queued-provisioning/gke-tpu-v6e/gke-tpu-v6e-deployment.yaml) | [`dws-flex-start-queued-provisioning/gke-tpu-v6e/README.md`](./dws-flex-start-queued-provisioning/gke-tpu-v6e/README.md) |
| **DWS Flex Start + Compact Placement** | **H4D (`h4d-standard-192`)** | [`dws-flex-start-compact-placement/gke-h4d/gke-h4d.yaml`](./dws-flex-start-compact-placement/gke-h4d/gke-h4d.yaml) | [`dws-flex-start-compact-placement/gke-h4d/gke-h4d-deployment.yaml`](./dws-flex-start-compact-placement/gke-h4d/gke-h4d-deployment.yaml) | [`dws-flex-start-compact-placement/gke-h4d/README.md`](./dws-flex-start-compact-placement/gke-h4d/README.md) |

## Quick Usage

From the repository root, ensure `./gcluster` is built (`test -x ./gcluster || make`), inspect any blueprint with `./gcluster explain <blueprint.yaml>`, and deploy using its companion `-deployment.yaml` file:

```bash
./gcluster deploy \
  -d examples/gke-consumption-options/dws-flex-start/gke-a3-ultragpu-deployment.yaml \
  examples/gke-consumption-options/dws-flex-start/gke-a3-ultragpu.yaml
```
