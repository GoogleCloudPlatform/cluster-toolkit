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
| `bucket` | Name of the GCS bucket to store Terraform state. |

### Consumption Options

Option 1 (Specific Reservation) is uncommented by default in `gke-a3-highgpu-deployment.yaml`. To use another consumption model, comment out Option 1 and uncomment the desired option.

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

## DWS Flex Start

### Submit a job using DWS Flex Start

1. Submit the DWS Flex Start job:

    ```bash
    kubectl apply -f examples/dws-sample-workloads/sample-job-flex.yaml
    ```

2. Consider using `kubectl get jobs` and `kubectl describe job <job-name>` to get information about the jobs.\
    You can also use `kubectl get pods` and `kubectl describe pod <pod-name>` to get pod information.

3. Clean up the job:

    ```bash
    kubectl delete -f examples/dws-sample-workloads/sample-job-flex.yaml
    ```

*Note: DWS Flex Start workloads require `nodeSelector: cloud.google.com/gke-flex-start: "true"`.*

### Deploy the NCCL test JobSet for DWS Flex Start

1. Deploy the NCCL test JobSet:

    ```bash
    kubectl create -f examples/gke-a3-highgpu/nccl-jobset-flex.yaml
    ```

2. Monitor pods (`kubectl get pods`) and check results in the primary pod logs:

    ```bash
    kubectl logs <jobset-pod-name>
    ```

3. Clean up test resources:

    ```bash
    kubectl delete -f examples/gke-a3-highgpu/nccl-jobset-flex.yaml
    ```

## DWS Flex Start + Queued Provisioning

### Submit a job using Queued Provisioning

1. Submit the Queued Provisioning job:

    ```bash
    kubectl apply -f examples/dws-sample-workloads/sample-job-flex-queue.yaml
    ```

2. Consider using `kubectl get jobs` and `kubectl describe job <job-name>` to get information about the jobs.\
    You can also use `kubectl get pods` and `kubectl describe pod <pod-name>` to get pod information.

3. Clean up the job:

    ```bash
    kubectl delete -f examples/dws-sample-workloads/sample-job-flex-queue.yaml
    ```

*Note: Queued Provisioning workloads require the label `kueue.x-k8s.io/queue-name: dws-local-queue` and annotation `provreq.kueue.x-k8s.io/maxRunDurationSeconds`.*

### Deploy the NCCL test JobSet for DWS Flex Start with Queued Provisioning

1. Deploy the NCCL test JobSet:

    ```bash
    kubectl create -f examples/gke-a3-highgpu/nccl-jobset-flex-queue.yaml
    ```

2. Monitor pods (`kubectl get pods`) and check results in the primary pod logs:

    ```bash
    kubectl logs <jobset-pod-name>
    ```

3. Clean up test resources:

    ```bash
    kubectl delete -f examples/gke-a3-highgpu/nccl-jobset-flex-queue.yaml
    ```

## Clean Up

To avoid incurring charges for the resources created, destroy the deployment:

```bash
./gcluster destroy DEPLOYMENT_NAME
```

**Note:** GCS buckets created for Terraform state are not deleted by the `./gcluster destroy` command and must be deleted manually.
