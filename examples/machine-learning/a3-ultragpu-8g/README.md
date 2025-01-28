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
