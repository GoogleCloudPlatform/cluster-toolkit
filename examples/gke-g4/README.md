# GKE G4 Blueprint

This blueprint uses GKE to provision a Kubernetes cluster and a G4 node pool, along with networks and service accounts. Information about the G4 machines can be found [here](https://cloud.google.com/blog/products/compute/introducing-g4-vm-with-nvidia-rtx-pro-6000).

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

## Clean Up
To destroy all resources associated with creating the GKE cluster, run the following command:

```sh
./gcluster destroy CLUSTER-NAME
```

Replace `CLUSTER-NAME` with the `deployment_name` used in the blueprint vars block.
