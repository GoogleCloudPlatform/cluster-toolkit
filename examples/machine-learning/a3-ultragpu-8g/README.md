# A3 Ultra Blueprints

## Slurm compute clusters
For further information on deploying an A3 Ultra cluster with Slurm, please
see:

[Create A3 Ultra Slurm Cluster](https://cloud.google.com/ai-hypercomputer/docs/create/create-slurm-cluster)

### Selective Deployment and Destruction using --only and --skip flags

You can control which parts of a blueprint are deployed or destroyed using the `--only` and `--skip` flags with the `gcluster deploy` and `gcluster destroy` commands. This is useful for saving time by not acting on components unnecessarily or for more granular control over resources.

A3-Ultra blueprint is divided into logical groups. Common groups include `cluster-env`, `cluster`, `image-env`, and `image`. The exact groups available depend on the blueprint definition.

#### `--only <group1>,<group2>,...`

Use the `--only` flag to have the command act on *only* the specified, comma-separated groups. Other groups will be untouched.

**Examples:**

- Deploy only the `cluster-env` group:

    ```bash
    ./gcluster deploy -d a3ultra-slurm-deployment.yaml examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml --only cluster-env
    ```

- Destroy only the `image` group:

    ```bash
    ./gcluster destroy deployment-name --only image
    ```

- Deploy only the `cluster-env` and `cluster` groups:

    ```bash
    ./gcluster deploy -d a3ultra-slurm-deployment.yaml examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml --only cluster-env,cluster
    ```

#### `--skip <group1>,<group2>,...`

Use the `--skip` flag to have the command act on all groups *except* those specified in the comma-separated list.

**Examples:**

- Deploy everything *except* the `image` group:

    ```bash
    ./gcluster deploy -d a3ultra-slurm-deployment.yaml examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml --skip image
    ```

- Destroy everything *except* the `cluster-env` group:

    ```bash
    ./gcluster destroy deployment-name --skip cluster-env
    ```

**Use Cases:**

- **Faster Iteration:** When developing, only `deploy` the group you are modifying (e.g., `--only cluster-env`).
- **Partial Teardown:** Selectively `destroy` parts of a deployment without affecting others (e.g., `--only image` to remove image but keep networking and other things).
- **Avoiding Unchanged Parts:** Use `--skip` to not redeploy or destroy parts you know are stable or should be preserved (e.g., `--skip cluster,image`).
- **Retry Failed Operations:** If a `deploy` or `destroy` fails on a specific group, you can rerun the command targeting just that group using `--only`.

### Cloud Storage FUSE

This blueprint includes four Cloud Storage FUSE mounts to provide a simple and scalable way
to manage data.

1. `/gcs` is a general purpose mount that can be used for shared tools and data.
1. `/gcs-checkpoints` is an optimized mount for writing and reading checkpoints. It
    uses the local SSD for caching and enables parallel downloads to improve
    performance.
1. `/gcs-training-data` is an optimized mount for reading training data. It can
   be further tuned if the training data fits fully within the local ssd
   storage.
1. `/gcs-model-serving` is an optimized mount for serving models, which
   downloads model weights in parallel to local ssd.

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

We provide ways to enable the alternative provisioning models in the
`a3ultra-slurm-deployment.yaml` file.

To make use of these other models, replace `a3u_reservation_name` in the
deployment file with the variable of choice below.

`a3u_enable_spot_vm: true` for spot or `a3u_dws_flex_enabled: true` for DWS Flex-Start.
