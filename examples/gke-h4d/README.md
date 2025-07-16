# GKE H4D Blueprint

This blueprint uses GKE to provision a Kubernetes cluster and a H4D node pool, along with networks and service accounts. Information about H4D machines can be found [here](https://cloud.google.com/blog/products/compute/new-h4d-vms-optimized-for-hpc).

> **_NOTE:_** The required GKE version for H4D support is >= 1.32.3-gke.1170000.

## Steps to deploy the H4D blueprint

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

1. Update the vars block of the `gke-h4d-deployment.yaml` file.
    1. `project_id`: ID of the project where you are deploying the cluster.
    1. `deployment_name`: Name of the deployment.
    1. `region`: Compute region used for the deployment.
    1. `zone`: Compute zone used for the deployment.
    1. `static_node_count`: Number of nodes to create.
    1. `authorized_cidr`: update the IP address in `<your-ip-address>/32`.
1. Build the Cluster Toolkit binary

   ```sh
   make
   ```

1. Provision the GKE cluster

   ```sh
   ./gcluster deploy -d examples/gke-h4d/gke-h4d-deployment.yaml examples/gke-h4d/gke-h4d.yaml
   ```

   These four options are displayed:

   ```sh
   (D)isplay full proposed changes,
   (A)pply proposed changes,
   (S)top and exit,
   (C)ontinue without applying
   ```

   Type `a` and hit enter to create the cluster.

## Clean Up
To destroy all resources associated with creating the GKE cluster, run the following command:

```sh
./gcluster destroy CLUSTER-NAME
```

Replace `CLUSTER-NAME` with the `deployment_name` used in the blueprint vars block.
