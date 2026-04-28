# Deploy an A3 Mega GKE cluster for ML training

This blueprint provisions a Google Kubernetes Engine (GKE) cluster with A3 Mega nodes (`a3-megagpu-8g`). A3 Mega VMs feature 8 NVIDIA H100 GPUs and high-bandwidth networking.

The blueprint automatically configures the following components to enable optimal GPU performance and multi-networking:

- **GPU-Direct TCPXO**: Optimized networking stack for high-bandwidth, low-latency GPU communication, specifically designed for A3 Mega.
- **Multi-networking**: Configures 8 secondary interfaces (VPC networks) for dedicated GPU-to-GPU traffic.
- **NRI Device Injector**: Automatically injects required networking and GPU configurations into your ML containers.
- **Kueue and JobSet**: Kubernetes-native tools for managing large-scale, multi-node training jobs with Topology Aware Scheduling (TAS).

## Prerequisites

1. **Cluster Toolkit:** Ensure you have installed all the dependencies required in cluster toolkit and followed the setup instructions.
    1. Install [dependencies](https://docs.cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
    2. Set up [Cluster Toolkit](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment). For building the `gcluster` binary, see [Install Cluster Toolkit](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment#install).
2. **Quota**: Ensure you have sufficient quota for `a3-megagpu-8g` machines in your chosen region.
3. **IP Address**: You will need the public IP address of the machine where you run `gcluster` to configure the cluster's authorized networks.

## Configuration

Before deploying, fill out the `gke-a3-megagpu-deployment.yaml` file with your project-specific values:

| Variable | Description |
| :--- | :--- |
| `project_id` | Your Google Cloud Project ID. |
| `deployment_name` | A unique name for this Cluster Toolkit deployment. |
| `region` / `zone` | The GCP region and zone (e.g., `us-east5`, `us-east5-a`). |
| `authorized_cidr` | Your public IP address in CIDR notation (e.g., `1.2.3.4/32`). |
| `static_node_count` | Number of A3 Mega nodes to provision. |
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
        examples/gke-a3-megagpu/gke-a3-megagpu.yaml \
        -d examples/gke-a3-megagpu/gke-a3-megagpu-deployment.yaml
    ```

## Verify NCCL Performance

After deployment, you can verify the GPU and networking performance using the included NCCL test manifest:

1. Get cluster credentials:

    ```bash
    gcloud container clusters get-credentials DEPLOYMENT_NAME --region REGION --project PROJECT_ID
    ```

    Replace `DEPLOYMENT_NAME`, `REGION`, and `PROJECT_ID` with the ones used in your `gke-a3-megagpu-deployment.yaml` file.

2. Verify that the GPUDirect TCPXO driver installers and device injectors are running on all nodes.

    ```bash
    kubectl get daemonsets -n kube-system nccl-tcpxo-installer
    kubectl get pods -n kube-system | grep device-injector
    ```

    *Example Output:*

    ```text
    NAME                   DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
    nccl-tcpxo-installer   2         2         2       2            2           <none>          30h
    device-injector-dzblk                                      1/1     Running   0              30h
    device-injector-r44p5                                      1/1     Running   0              30h
    ```

3. Apply the NCCL test manifest:

    ```bash
    kubectl apply -f examples/gke-a3-megagpu/nccl-test-latest.yaml
    ```

4. Wait for the pods to be ready:

    ```bash
    kubectl get pods
    ```

    *Example Output:*

    ```text
    NAME               READY   STATUS    RESTARTS   AGE
    nccl-test-host-1   2/2     Running   0          17s
    nccl-test-host-2   2/2     Running   0          17s
    ```

5. Run the benchmark:

    ```bash
    kubectl exec nccl-test-host-1 -c nccl-test -- /scripts/allgather.sh nccl-host-1 nccl-host-2
    ```

    *Example Output:*

    ```text
    #                                                              out-of-place                       in-place          
    #     size         count      type   redop    root     time   algbw   busbw  #wrong     time   algbw   busbw  #wrong 
    #     (B)    (elements)                               (us)  (GB/s)  (GB/s)             (us)  (GB/s)  (GB/s)         
           0             0     float    none      -1     0.30    0.00    0.00       0     0.26    0.00    0.00       0
           0             0     float    none      -1     0.26    0.00    0.00       0     0.25    0.00    0.00       0
           0             0     float    none      -1     0.27    0.00    0.00       0     0.25    0.00    0.00       0
           0             0     float    none      -1     0.25    0.00    0.00       0     0.30    0.00    0.00       0
           0             0     float    none      -1     0.24    0.00    0.00       0     0.25    0.00    0.00       0
         256             4     float    none      -1    86.95    0.00    0.00       0    88.21    0.00    0.00       0
         512             8     float    none      -1    85.70    0.01    0.01       0    84.64    0.01    0.01       0
        1024            16     float    none      -1    85.27    0.01    0.01       0    86.81    0.01    0.01       0
        2048            32     float    none      -1    84.50    0.02    0.02       0    84.02    0.02    0.02       0
        4096            64     float    none      -1    85.55    0.05    0.04       0    84.46    0.05    0.05       0
        8192           128     float    none      -1    84.25    0.10    0.09       0    84.08    0.10    0.09       0
       16384           256     float    none      -1    84.41    0.19    0.18       0    84.81    0.19    0.18       0
       32768           512     float    none      -1    92.99    0.35    0.33       0    95.44    0.34    0.32       0
       65536          1024     float    none      -1    96.98    0.68    0.63       0    93.80    0.70    0.66       0
      131072          2048     float    none      -1    98.33    1.33    1.25       0   112.25    1.17    1.09       0
      262144          4096     float    none      -1   129.67    2.02    1.90       0   130.32    2.01    1.89       0
      524288          8192     float    none      -1   126.01    4.16    3.90       0   127.00    4.13    3.87       0
     1048576         16384     float    none      -1   133.87    7.83    7.34       0   128.95    8.13    7.62       0
     2097152         32768     float    none      -1   136.34   15.38   14.42       0   133.81   15.67   14.69       0
     4194304         65536     float    none      -1   148.45   28.25   26.49       0   148.47   28.25   26.49       0
     8388608        131072     float    none      -1   166.11   50.50   47.34       0   161.13   52.06   48.81       0
    16777216        262144     float    none      -1   197.88   84.79   79.49       0   196.38   85.43   80.09       0
    33554432        524288     float    none      -1   290.54  115.49  108.27       0   284.11  118.10  110.72       0
    67108864       1048576     float    none      -1   517.87  129.59  121.49       0   444.29  151.05  141.61       0
    134217728       2097152     float    none      -1   806.59  166.40  156.00       0   790.70  169.74  159.14       0
    268435456       4194304     float    none      -1  1510.98  177.66  166.55       0  1507.46  178.07  166.94       0
    536870912       8388608     float    none      -1  2880.08  186.41  174.76       0  2872.37  186.91  175.23       0
    1073741824      16777216     float    none      -1  5531.01  194.13  182.00       0  5489.29  195.61  183.38       0
    2147483648      33554432     float    none      -1  10820.2  198.47  186.07       0  10805.3  198.74  186.32       0
    4294967296      67108864     float    none      -1  21430.5  200.41  187.89       0  21413.4  200.57  188.04       0
    8589934592     134217728     float    none      -1  42668.7  201.32  188.73       0  42630.7  201.50  188.90       0
    # Out of bounds values : 0 OK
    # Avg bus bandwidth    : 53.8932 
    ```

6. When you have finished validating the nodes, clean up the test resources so the nodes can be used by other workloads:

    ```bash
    kubectl delete -f examples/gke-a3-megagpu/nccl-test-latest.yaml
    ```

## Clean Up

To avoid incurring charges for the resources created, destroy the deployment:

```bash
./gcluster destroy DEPLOYMENT_NAME
```

## Additional Resources

Refer to [Deploy an A3 Mega GKE cluster for ML training](https://cloud.google.com/cluster-toolkit/docs/deploy/deploy-a3-mega-gke-cluster) for more instructions on creating the GKE-A3M cluster.

Refer to [Deploy and run NCCL test with Topology Aware Scheduling (TAS)](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#deploy-run-nccl-tas-test) for more instructions on running a NCCL test on the GKE-A3M cluster.

### Additional Consumption Options
The Cluster Toolkit supports alternative consumption options such as Spot VMs or Dynamic Workload Scheduler (DWS) Flex-start.
Refer to step 5 of [Create a cluster using Cluster Toolkit](https://docs.cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#use-cluster-toolkit) for general instructions on other consumption options. Similar configuration settings can be used for GKE-A3M cluster as well.
