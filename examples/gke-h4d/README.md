# GKE H4D Blueprint

This blueprint uses GKE to provision a Kubernetes cluster and a H4D node pool, along with networks and service accounts. Information about H4D machines can be found [here](https://cloud.google.com/blog/products/compute/new-h4d-vms-optimized-for-hpc).

> **_NOTE:_** The required GKE version for H4D support is >= 1.32.3-gke.1170000.

## Steps to deploy the H4D blueprint
Refer to [Run high performance computing (HPC) workloads with H4D](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/run-hpc-workloads#cluster-toolkit) for instructions on creating the GKE-H4D cluster.

## Run a test using the MPI Operator
The MPI Operator is installed on the cluster during the deployment. To run a test using the MPI Operator on the GKE H4D cluster, refer to https://github.com/GoogleCloudPlatform/kubernetes-engine-samples/tree/main/hpc/mpi.

## Clean Up
To destroy all resources associated with creating the GKE cluster, run the following command:

```sh
./gcluster destroy CLUSTER-NAME
```

Replace `CLUSTER-NAME` with the `deployment_name` used in the blueprint vars block.
