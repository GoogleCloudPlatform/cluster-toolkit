# GKE A4X Example

This example provides the configuration to deploy a GKE cluster with A4X machine types.

Refer to [Create an AI-optimized GKE cluster with default configuration](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#use-cluster-toolkit) for instructions on creating the GKE-A4X cluster.

Refer to [Deploy and run NCCL test with Topology Aware Scheduling (TAS)](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#deploy-run-nccl-tas-test) for instructions on running a NCCL test on the GKE-A4X cluster.

## Install and Run MPI Operator on GKE Cluster

The Kubeflow MPI Operator manages distributed MPI workloads on GKE.

1. **Deploy MPI Operator (v0.8.2):**

   * **Automated (During Cluster Creation via Blueprint YAML):** Include the MPI Operator manifest in `apply_manifests` under `kubectl-apply` in your blueprint YAML (`gke-a4x.yaml`):

     ```yaml
       - id: kubectl-apply
         source: modules/management/kubectl-apply
         use: [a4x-cluster]
         settings:
           apply_manifests:
           - name: mpi-operator
             source: https://raw.githubusercontent.com/kubeflow/mpi-operator/v0.8.2/deploy/v2beta1/mpi-operator.yaml
     ```

   * **Manual (After Cluster Deployment via `kubectl`):** Once the cluster is deployed, run the following command against your cluster:

     ```bash
     kubectl apply --server-side -f https://raw.githubusercontent.com/kubeflow/mpi-operator/v0.8.2/deploy/v2beta1/mpi-operator.yaml
     ```

2. **Verify Installation:**

   ```bash
   kubectl get crd | grep mpijob
   kubectl get pods -n mpi-operator
   ```

3. **Run a Sample MPIJob Test:**
   Create a test manifest `sample-mpijob.yaml`:

   ```yaml
   apiVersion: kubeflow.org/v2beta1
   kind: MPIJob
   metadata:
     name: sample-mpi-job
     namespace: default
   spec:
     slotsPerWorker: 1
     runPolicy:
       cleanPodPolicy: Running
     mpiReplicaSpecs:
       Launcher:
         replicas: 1
         template:
           spec:
             containers:
             - name: mpi-launcher
               image: mpioperator/mpi-pi:v0.8.2-openmpi
               command:
               - mpirun
               - --allow-run-as-root
               - -n
               - "2"
               - --hostfile
               - /etc/mpi/hostfile
               - echo
               - "Hello World from MPI worker!"
       Worker:
         replicas: 2
         template:
           spec:
             containers:
             - name: mpi-worker
               image: mpioperator/mpi-pi:v0.8.2-openmpi
   ```

   Submit the job and inspect launcher logs:

   ```bash
   kubectl apply -f sample-mpijob.yaml
   kubectl logs -l training.kubeflow.org/job-role=launcher
   kubectl delete -f sample-mpijob.yaml
   ```
