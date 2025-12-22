# A3 Mega / Slurm Blueprint for Google Cloud

To deploy an a3-megagpu-8g cluster running Slurm on Google Cloud, please follow
these [instructions].

[instructions]: https://cloud.google.com/cluster-toolkit/docs/deploy/deploy-a3-mega-cluster

## Selective Deployment and Destruction using --only and --skip flags

You can control which parts of a blueprint are deployed or destroyed using the `--only` and `--skip` flags with the `gcluster deploy` and `gcluster destroy` commands. This is useful for saving time by not acting on components unnecessarily or for more granular control over resources.

A3-Mega blueprint is divided into logical groups. Common groups include `cluster-env`, `cluster`, `image-env`, and `image`. The exact groups available depend on the blueprint definition.

### `--only <group1>,<group2>,...`

Use the `--only` flag to have the command act on *only* the specified, comma-separated groups. Other groups will be untouched.

**Examples:**

* Deploy only the `cluster-env` group:

    ```bash
    ./gcluster deploy -d a3mega-slurm-deployment.yaml examples/machine-learning/a3-megagpu-8g/a3mega-slurm-blueprint.yaml --only cluster-env
    ```

* Destroy only the `image` group:

    ```bash
    ./gcluster destroy deployment-name --only image
    ```

* Deploy only the `cluster-env` and `cluster` groups:

    ```bash
    ./gcluster deploy -d a3mega-slurm-deployment.yaml examples/machine-learning/a3-megagpu-8g/a3mega-slurm-blueprint.yaml --only cluster-env,cluster
    ```

### `--skip <group1>,<group2>,...`

Use the `--skip` flag to have the command act on all groups *except* those specified in the comma-separated list.

**Examples:**

* Deploy everything *except* the `image` group:

    ```bash
    ./gcluster deploy -d a3mega-slurm-deployment.yaml examples/machine-learning/a3-megagpu-8g/a3mega-slurm-blueprint.yaml --skip image
    ```

* Destroy everything *except* the `cluster-env` group:

    ```bash
    ./gcluster destroy deployment-name --skip cluster-env
    ```

**Use Cases:**

* **Faster Iteration:** When developing, only `deploy` the group you are modifying (e.g., `--only cluster-env`).
* **Partial Teardown:** Selectively `destroy` parts of a deployment without affecting others (e.g., `--only image` to remove image but keep networking and other things).
* **Avoiding Unchanged Parts:** Use `--skip` to not redeploy or destroy parts you know are stable or should be preserved (e.g., `--skip cluster,image`).
* **Retry Failed Operations:** If a `deploy` or `destroy` fails on a specific group, you can rerun the command targeting just that group using `--only`.

## GCSFuse with Local SSD cache

`a3mega-slurm-gcsfuse-lssd-blueprint.yaml` reflects best practices for using GCSFuse for ML workloads. It is configured to mount GCS buckets on two mountpoints on a3-mega nodes. Use the `gcs_bucket` variable to specify a GCS bucket to mount, or leave the variable empty to mount all available buckets [dynamically](https://cloud.google.com/storage/docs/cloud-storage-fuse/mount-bucket#dynamic-mount).
The `/gcs` mountpoint enables parallel downloads, intended for reading/writing checkpoints, logs, application outputs, model serving, or loading large files (e.g. squashfs files). The read-only `/gcs-ro` mountpoint disables parallel downloads and enables the list cache, intended for reading training data. Parallel downloads are not recommended for training workloads; see [GCSFuse documentation](https://cloud.google.com/storage/docs/cloud-storage-fuse/file-caching#parallel-downloads) for details.
