# Deploy an A3 High GKE cluster for ML training

This blueprint provisions a Google Kubernetes Engine (GKE) cluster with A3 High nodes (`a3-highgpu-8g`). A3 High VMs feature 8 NVIDIA H100 GPUs and 200 Gbps of networking throughput per GPU.

The blueprint automatically configures the following components to enable optimal GPU performance and multi-networking:

- **GPU-Direct TCPX**: Optimized networking stack for high-bandwidth, low-latency GPU communication.
- **Multi-networking**: Configures 4 secondary interfaces (VPC networks) for dedicated GPU-to-GPU traffic.
- **NRI Device Injector**: Automatically injects required networking and GPU configurations into your ML containers.
- **Kueue and JobSet**: Kubernetes-native tools for managing large-scale, multi-node training jobs with Topology Aware Scheduling (TAS).

## Prerequisites

1. **Cluster Toolkit:** Ensure you have installed all the dependencies required in cluster toolkit and followed the setup instructions.
    1. Install [dependencies](https://docs.cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
    2. Set up [Cluster Toolkit](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment). For building the `gcluster` binary, see [Install Cluster Toolkit](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment#install).
2. **Quota**: Ensure you have sufficient quota for `a3-highgpu-8g` machines in your chosen region.
3. **IP Address**: You will need the public IP address of the machine where you run `gcluster` to configure the cluster's authorized networks.

## Configuration

Before deploying, fill out the `gke-a3-highgpu-deployment.yaml` file with your project-specific values:

| Variable | Description |
| :--- | :--- |
| `project_id` | Your Google Cloud Project ID. |
| `deployment_name` | A unique name for this Cluster Toolkit deployment. |
| `region` / `zone` | The GCP region and zone (e.g., `us-central1`, `us-central1-c`). |
| `authorized_cidr` | Your public IP address in CIDR notation (e.g., `1.2.3.4/32`). |
| `static_node_count` | Number of A3 High nodes to provision. |
| `reservation` | (Optional) The name of a GCE reservation to use. |
| `bucket` | Name of the GCS bucket to store Terraform state. |

## Deploy the Cluster

1. Switch to the toolkit directory:

    ```bash
    cd ~/cluster-toolkit
    ```

2. Build the toolkit:

    ```bash
    make
    ```

3. Deploy the infrastructure:

    ```bash
    ./gcluster deploy \
        examples/gke-a3-highgpu/gke-a3-highgpu.yaml \
        -d examples/gke-a3-highgpu/gke-a3-highgpu-deployment.yaml
    ```

## Verify NCCL Performance

Refer the following guide to verify GPU and networking performance using NVIDIA `nccl-tests`:

- **Multi-Node (16+ GPUs)**: [Multi-Node Test Plan](multi-node-test-plan.md)

## Clean Up

To avoid incurring charges for the resources created, destroy the deployment:

```bash
./gcluster destroy DEPLOYMENT_NAME
```
