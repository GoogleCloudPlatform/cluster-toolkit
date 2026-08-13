# A3 Mega / Slurm Blueprint for Google Cloud

To deploy an a3-megagpu-8g cluster running Slurm on Google Cloud, please follow
these [instructions].

[instructions]: https://cloud.google.com/cluster-toolkit/docs/deploy/deploy-a3-mega-cluster

Selective deployment and teardown for this blueprint are documented centrally. See [examples/machine-learning/README.md](../README.md) for full details.

Example (deploy only the primary group for this blueprint):

```bash
./gcluster deploy -d a3mega-slurm-deployment.yaml a3mega-slurm-blueprint.yaml --only primary
```

## GCSFuse with Local SSD cache

`a3mega-slurm-gcsfuse-lssd-blueprint.yaml` reflects best practices for using GCSFuse for ML workloads. It is configured to mount GCS buckets on two mountpoints on a3-mega nodes. Use the `gcs_bucket` variable to specify a GCS bucket to mount, or leave the variable empty to mount all available buckets [dynamically](https://cloud.google.com/storage/docs/cloud-storage-fuse/mount-bucket#dynamic-mount).
The `/gcs` mountpoint enables parallel downloads, intended for reading/writing checkpoints, logs, application outputs, model serving, or loading large files (e.g. squashfs files). The read-only `/gcs-ro` mountpoint disables parallel downloads and enables the list cache, intended for reading training data. Parallel downloads are not recommended for training workloads; see [GCSFuse documentation](https://cloud.google.com/storage/docs/cloud-storage-fuse/file-caching#parallel-downloads) for details.
