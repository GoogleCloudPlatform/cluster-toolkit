# A3 High Multi-Node Test Plan

This document outlines the step-by-step instructions to validate a multi-node Google Kubernetes Engine (GKE) cluster for A3 High instances powered by NVIDIA H100 80GB GPUs.

## Prerequisites

Connect to your cluster:

```bash
gcloud container clusters get-credentials CLUSTER_NAME \
    --location=COMPUTE_REGION
```

Replace the following variables:

- `CLUSTER_NAME`: the name of your cluster, which, for the clusters created with Cluster Toolkit, is based on the `DEPLOYMENT_NAME`.
- `COMPUTE_REGION`: the name of the compute region.

Verify that `kueue` components are deployed effectively if using local queues.

## Step 1: Verify TCPX Components

Verify that the GPUDirect TCPX driver installers and device injectors are running on all nodes.

```bash
# Verify TCPX DaemonSets are fully deployed to the cluster nodes
kubectl get daemonsets -n kube-system nccl-tcpx-installer
kubectl get pods -n kube-system | grep device-injector
```

*Example Output:*

```text
NAME                  DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
nccl-tcpx-installer   2         2         2       2            2           <none>          18h
device-injector-fhkn4                                      1/1     Running   0             15h
device-injector-w5d4c                                      1/1     Running   0             15h
```

## Step 2: Ensure Node Queues are Ready

Kueue manages batch dispatching for the local queues. Verify that they are active so the NCCL tests can be picked up.

```bash
kubectl get clusterqueue
kubectl get localqueue
```

## Step 3: Configure and Apply the NCCL Test Manifest

We use the file `examples/gke-a3-highgpu/nccl-test.yaml` for a multi-node test. By default, this file contains the definitions for 2 nodes (`nccl-test-host-1` and `nccl-test-host-2`). Make the necessary changes to this YAML file if you want to use a number of nodes other than 2 (by adding additional Pods and Services).

> [!NOTE]
> **Testing > 2 Nodes:**
> If your cluster has more than 2 nodes and you wish to run the NCCL test across all of them, you must manually update `examples/gke-a3-highgpu/nccl-test.yaml` by duplicating the definitions:
>
> 1. Duplicate the `Service` and `Pod` YAML blocks for `nccl-host-2` and `nccl-test-host-2`.
> 2. Paste them at the end of the file, incrementing the names (e.g., `nccl-host-3` / `nccl-test-host-3`, `nccl-host-4` / `nccl-test-host-4`), up to the number of nodes you want to test.

If you want to test nodes provisioned by Flex Start, you must add the max run duration annotation to the `metadata.annotations` section of **every** `Pod` defined in `examples/gke-a3-highgpu/nccl-test.yaml`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nccl-test-host-1
  annotations:
    provreq.kueue.x-k8s.io/maxRunDurationSeconds: "600"
    # ... existing annotations ...
```

Deploy the Test:

```bash
kubectl apply -f examples/gke-a3-highgpu/nccl-test.yaml
```

Wait for the Pods to start:

```bash
kubectl get pods
```

*Example Output:*

```text
NAME               READY   STATUS    RESTARTS   AGE
nccl-test-host-1   2/2     Running   0          7m46s
nccl-test-host-2   2/2     Running   0          7m45s
```

You should see all `nccl-test-host-N` pods reach the `Running` state (Wait until `READY` says `2/2`).

## Step 4: Run the Benchmark and Validate Results

Once both pods are `Running`, you can execute the test suite (which includes `allgather`) by triggering the script on the first host pod.

1. **Execute the test through `nccl-test-host-1`:**

    ```bash
    kubectl exec -t nccl-test-host-1 -c nccl-test -- /bin/bash -c "cp /configs/allgather.sh /scripts/allgather.sh"
    kubectl exec -t nccl-test-host-1 -c nccl-test -- /scripts/allgather.sh nccl-host-1 nccl-host-2
    ```

    > [!TIP]
    > **Testing > 2 Nodes:** If you modified `nccl-test.yaml` to include more nodes (e.g., `nccl-host-3`, `nccl-host-4`), make sure to append their hostnames to the argument list above:
    > `... /scripts/allgather.sh nccl-host-1 nccl-host-2 nccl-host-3 nccl-host-4`

    The output will stream to your terminal as the MPI workers synchronize across the nodes.

2. **Verify Results:**
    Wait for the benchmark tables to print to your console. Look for the `busbw (GB/s)` column to ensure TCPX networking and the secondary NICs are successfully scaling multi-node traffic.

    *Example Output:*

    ```text
    nccl-test-host-1:166:207 [0] NCCL INFO NCCL_P2P_PXN_LEVEL set by environment to 0.
    #
    #                                                              out-of-place                       in-place          
    #       size         count      type   redop    root     time   algbw   busbw #wrong     time   algbw   busbw #wrong
    #        (B)    (elements)                               (us)  (GB/s)  (GB/s)            (us)  (GB/s)  (GB/s)       
         1048576         16384     float    none      -1    996.7    1.05    0.99    N/A   1007.1    1.04    0.98    N/A
         2097152         32768     float    none      -1    988.6    2.12    1.99    N/A    973.5    2.15    2.02    N/A
         4194304         65536     float    none      -1    942.9    4.45    4.17    N/A    935.8    4.48    4.20    N/A
         8388608        131072     float    none      -1   1007.2    8.33    7.81    N/A   1018.2    8.24    7.72    N/A
        16777216        262144     float    none      -1   1063.4   15.78   14.79    N/A   1057.9   15.86   14.87    N/A
        33554432        524288     float    none      -1   1116.5   30.05   28.18    N/A   1220.1   27.50   25.78    N/A
        67108864       1048576     float    none      -1   1334.7   50.28   47.14    N/A   1359.5   49.36   46.28    N/A
       134217728       2097152     float    none      -1   1940.8   69.15   64.83    N/A   1940.9   69.15   64.83    N/A
       268435456       4194304     float    none      -1   3624.3   74.07   69.44    N/A   3545.2   75.72   70.99    N/A
       536870912       8388608     float    none      -1   7098.6   75.63   70.90    N/A   7054.3   76.11   71.35    N/A
    # Out of bounds values : 0 OK
    # Avg bus bandwidth    : 30.9622 
    ```

For multi-node H100 NCCL workloads over TCPX, you should observe optimized GB/s scaling without hangs or segmentation faults.

## Step 5: Clean Up

When you have finished validating the nodes, clean up the test resources so the nodes can be used by other workloads:

```bash
kubectl delete -f examples/gke-a3-highgpu/nccl-test.yaml
```
