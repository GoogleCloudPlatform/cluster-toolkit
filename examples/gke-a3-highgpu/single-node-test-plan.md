# A3 High Single-Node Test Plan

This test plan outlines the steps required to verify the successful deployment of a single-node A3 High GKE cluster, specifically focusing on validating Kueue, TCPX, NCCL, and the NRI device injector.

## Prerequisites

- The single-node A3 High deployment must be completely provisioned and in an `ACTIVE` state.
- `kubectl` must be installed and configured to point to the newly created deployment.

## 1. Verify Node Resource Injector (NRI)
The NRI is responsible for intercepting container creations and injecting device configurations, such as pointing the container to the correct network interfaces.

***Action:** Check that the NRI DaemonSet is running in the `kube-system` namespace.
    ```bash
    kubectl get pods -n kube-system | grep device-injector
    ```
***Action:** Verify the NRI pod logs to confirm successful registration with containerd.
    ```bash
    kubectl logs -n kube-system -l k8s-app=device-injector
    ```
    *Success Criteria:* The `nccl-test-single-node` pod uses the annotations to mount the TCPX devices. Check the NRI device injector logs to ensure it successfully annotated and injected devices like `/dev/nvidia0` or `/dev/nvidiactl` when the pod started.

    *Example Output:*
    ```text
    time="2026-03-12T11:26:30Z" level=info msg="Annotated device" container=tcpx-daemon device=/dev/nvidia7 namespace=default pod=nccl-test-single-node
    time="2026-03-12T11:26:30Z" level=info msg="Injected device" container=tcpx-daemon device=/dev/nvidia7 namespace=default pod=nccl-test-single-node
    time="2026-03-12T11:26:30Z" level=info msg="Annotated device" container=tcpx-daemon device=/dev/nvidiactl namespace=default pod=nccl-test-single-node
    time="2026-03-12T11:26:30Z" level=info msg="Injected device" container=tcpx-daemon device=/dev/nvidiactl namespace=default pod=nccl-test-single-node
    ```

## 2. Verify TCPX Network Interfaces
Even on a single node, the TCPX drivers and `gpunets` configurations must be applied properly to the host for future multi-node scaling or proper GPU networking isolation.

***Action:** Check if the `nccl-tcpx-installer` DaemonSet ran successfully to install the drivers on the host.
    ```bash
    kubectl get pods -n kube-system | grep nccl-tcpx-installer
    ```
    *Example Output:*
    ```text
    nccl-tcpx-installer-r5cmw                                 1/1     Running   0             23h
    ```

***Action:** Verify the NCCL TCPX drivers are correctly applied to the GPUs.
    ```bash
    kubectl get daemonset -n kube-system | grep nccl
    ```
    *Example Output:*
    ```text
    nccl-fastsocket-installer                1         1         1       1            1           <none>                                                               24h
    nccl-tcpx-installer                      1         1         1       1            1           <none>                                                               24h
    ```
    *Success Criteria:* You should observe both `nccl-tcpx-installer` and `nccl-fastsocket-installer` shown as running.

***Action:** Verify the four secondary GPU network interfaces are physically attached and allocated to the node.
    ```bash
    kubectl describe node -l cloud.google.com/gke-accelerator=nvidia-h100-80gb | grep -i networking.gke.io.networks/vpc
    ```
    *Success Criteria:* You should see output indicating that `vpc1` through `vpc4` are allocated to the node (e.g., `networking.gke.io.networks/vpc1: 1`).

## 3. Verify Kueue Orbit (Job Orchestration)
Ensure the Kueue batch scheduler controller is active and the queues defined in the blueprint were successfully created.

***Action:** Verify the Kueue controller pods are running gracefully.
    ```bash
    kubectl get pods -n kueue-system
    ```
***Action:** Verify the queues are defined and active.
    ```bash
    kubectl get clusterqueues
    # Expected: Should list the cluster-queue

    kubectl get localqueues
    # Expected: Should list main-queue and report Active: True
    ```

## 4. Test Single-Node NCCL (Intra-Node Communication)
Run the standard NVIDIA `nccl-tests` suite to verify communication across the NVLink hardware bridge connecting the 8 GPUs within the single physical machine. The test pod runs a suite of three distinct NCCL benchmarks in a continuous loop:
- `all_reduce_perf`
- `all_gather_perf`
- `reduce_scatter_perf`

***Action:** Ensure a clean slate by deleting any existing test pod:
    ```bash
    kubectl delete pod nccl-test-single-node --force --ignore-not-found
    ```
***Action:** Apply the pre-built Kueue Job YAML file from the cluster-toolkit examples directory:
    ```bash
    kubectl apply -f /usr/local/google/home/shubpal/cluster-toolkit/examples/gke-a3-highgpu/nccl-test-single-node.yaml
    ```
***Action:** Watch Kueue admit the pod to the cluster.
    ```bash
    kubectl wait --for=condition=Ready pod/nccl-test-single-node --timeout=300s
    ```
***Action:** Inspect the pod logs to verify performance across all tests.
    ```bash
    kubectl exec -t nccl-test-single-node -c nccl-test -- cat /tmp/nccl_results.txt | grep -A 10 "size"
    ```
    *Success Criteria:* The `BusBw (GB/s)` column in the output for the largest payload sizes should reach **250+ GB/s** or higher. This confirms that the NVLink fabric is working optimally across all 8 GPUs on the node via NCCL.

    *Example Output:*
    ```text
    #                                                              out-of-place                       in-place          
    #       size         count      type   redop    root     time   algbw   busbw #wrong     time   algbw   busbw #wrong
    #        (B)    (elements)                               (us)  (GB/s)  (GB/s)            (us)  (GB/s)  (GB/s)       
      1073741824     268435456     float     sum      -1   7151.5  150.14  262.75    N/A   7143.7  150.31  263.03    N/A
      2147483648     536870912     float     sum      -1    14301  150.17  262.79    N/A    14277  150.41  263.22    N/A
      4294967296    1073741824     float     sum      -1    28566  150.35  263.12    N/A    28544  150.47  263.32    N/A
      8589934592    2147483648     float     sum      -1    57108  150.42  263.23    N/A    57038  150.60  263.55    N/A
    ```
