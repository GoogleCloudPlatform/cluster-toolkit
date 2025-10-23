# GKE G4 Blueprint

This blueprint uses GKE to provision a Kubernetes cluster and a G4 node pool, along with networks and service accounts. More information about G4 machines can be found here:

* [Blog post](https://cloud.google.com/blog/products/compute/introducing-g4-vm-with-nvidia-rtx-pro-6000)
* [Documentation](https://cloud.google.com/compute/docs/gpus#rtx-6000-gpus)

> **_NOTE:_** The required GKE version for G4 support is >= 1.32.4-gke.1698000.

## Steps to deploy the G4 blueprint

1. Install Cluster Toolkit
    1. Install [dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
    1. Set up [Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).
1. Switch to the Cluster Toolkit directory

   ```sh
   cd cluster-toolkit
   ```

1. Get the IP address for your host machine

   ```sh
   curl ifconfig.me
   ```

1. Update the vars block of the `gke-g4-deployment.yaml` file.
    1. `project_id`: ID of the project where you are deploying the cluster.
    1. `deployment_name`: Name of the deployment.
    1. `region`: Compute region used for the deployment.
    1. `zone`: Compute zone used for the deployment.
    1. `machine_type`: The VM shape. See allowed values at https://cloud.google.com/compute/docs/gpus#rtx-6000-gpus.
    1. `num_gpus`: Number of GPUS in the VM. Can be found at https://cloud.google.com/compute/docs/gpus#rtx-6000-gpus.
    1. `static_node_count`: Number of nodes to create.
    1. `authorized_cidr`: update the IP address in `<your-ip-address>/32`.
1. Build the Cluster Toolkit binary

   ```sh
   make
   ```

1. Provision the GKE cluster

   ```sh
   ./gcluster deploy -d examples/gke-g4/gke-g4-deployment.yaml examples/gke-g4/gke-g4.yaml
   ```

   These four options are displayed:

   ```sh
   (D)isplay full proposed changes,
   (A)pply proposed changes,
   (S)top and exit,
   (C)ontinue without applying
   ```

   Type `a` and hit enter to create the cluster.

## NCCL Tests for GKE G4

This directory contains a manifest to run NVIDIA NCCL performance tests on the GKE G4 cluster.

### Overview

As RDMA networking and the Google gIB plugin are not supported for G4 machines, the G4 instances use standard TCP/IP networking. The NCCL test provided here is configured to build from source. It uses the `nvidia/cuda` development image to clone and compile `nccl-tests` at runtime, ensuring the latest compatible tests are run.

### Running the Test

1. **Deploy the GKE G4 Cluster:**
    Ensure you have deployed the cluster using the `gke-g4` blueprint.

2. **Configure the Test Manifest:**
   Open `nccl-test.yaml` and update the following fields to match your cluster configuration:
   * `cloud.google.com/gke-nodepool`: Ensure this matches your deployed nodepool name (default in blueprint is `g4-standard-96-g4-pool`).
   * `nvidia.com/gpu` (limits/requests): Set this to the number of GPUs on your node (e.g., 1, 4, 8, etc.).
   * Command argument `-g 2`: Update the `-g` flag in the command to match the number of GPUs.
   * `NCCL_P2P_LEVEL`: Update this to "SYS" if using 8-GPU g4-standard-384 machines. Else should remain as "PHB".

3. **Apply the Job:**

   ```bash
   kubectl apply -f examples/gke-g4/nccl-test.yaml
   ```

4. **View Results:**
   Wait for the job to complete, then check the logs:

   ```bash
   # Find the pod name
   kubectl get pods
    
   # View logs
   kubectl logs <POD_NAME>
   ```

   You should see output indicating the bus bandwidth achieved during the `all_reduce_perf` test.

## Clean Up
To destroy all resources associated with creating the GKE cluster, run the following command:

```sh
./gcluster destroy CLUSTER-NAME
```

Replace `CLUSTER-NAME` with the `deployment_name` used in the blueprint vars block.
