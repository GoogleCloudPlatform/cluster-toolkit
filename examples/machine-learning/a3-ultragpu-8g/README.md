# A3 Ultra Blueprints

## Slurm compute clusters
For further information on deploying an A3 Ultra cluster with Slurm, please
see:

[Create A3 Ultra Slurm Cluster](https://cloud.google.com/ai-hypercomputer/docs/create/create-slurm-cluster)

## VMs without scheduler

To test workloads directly on A3 Ultra VMs, you can deploy the [a3ultra-vm.yaml]:

- A configurable number of A3 Ultra VMs (default N=2)
- RDMA networking and GPU drivers pre-configured on our Ubuntu 22.04 Accelerator Image
- Additional software environment customization can be achieved by adding to the example startup-script

The VMs can be consumed from a reservation by modifying the `reservation_name` parameter in the `a3ultra-vms` module.

[a3ultra-vm.yaml]: ./a3ultra-vm.yaml

### Additional ways to provision
Cluster toolkit also supports DWS Flex-Start, Spot VMs, as well as reservations as ways to provision instances.

[For more information on DWS Flex-Start in Slurm](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/slurm-dws-flex.md)
[For more information on Spot VMs](https://cloud.google.com/compute/docs/instances/spot)

We provide ways to enable the alternative provisioning models in the `a3ultra-slurm-deployment.yaml` file.

To make use of these other models, replace `a3u_reservation_name` in the deployment file with the variable of choice below.

`a3u_enable_spot_vm: true` for spot or `a3u_dws_flex_enabled: true` for DWS Flex-Start.
