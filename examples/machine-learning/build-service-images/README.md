# Build Service Images

This process will build 2 images. One for a3-megagpu-8g and another one that
can be used for all other machines. Each image it may take up to ~60 min to
build. You may choose to run the two commands in parallel in 2 separate shells.

## Requirements

Make sure `gcluster`, `gcloud`, `terraform` and `packer`
binaries to be available in `PATH`.

## Build A3M image

Run the following commands,

```sh
cd cluster-toolkit/examples/machine-learning/build-service-images/
PROJECT=<your-project-id>
SUFFIX=slurm-image
./build.sh a3m $SUFFIX $PROJECT
```

## Build common image

Run the following commands,

```sh
cd cluster-toolkit/examples/machine-learning/build-service-images/
PROJECT=<your-project-id>
SUFFIX=slurm-image
./build.sh common $SUFFIX $PROJECT
```

## Next Steps

The two images that were build can be found under `*-slurm-image` family. Use the following
gcloud command to describe the images and confirm they were built.

```shell
gcloud compute images describe-from-family a3m-slurm-image
gcloud compute images describe-from-family common-slurm-image
```

## Troubleshooting

If packer fails during execution of startup script it will output a
command for grabbing logs, e.g.:

```shell
==> roll.googlecompute.toolkit_image: Error waiting for startup script to finish: Startup script exited with error.
==> roll.googlecompute.toolkit_image: Provisioning step had errors: Running the cleanup provisioner, if present...
==> roll.googlecompute.toolkit_image: Running local shell script: /tmp/packer-shell2734275589
    roll.googlecompute.toolkit_image: Error building image try checking logs:
    roll.googlecompute.toolkit_image: gcloud logging --project <project-id> read 'logName=("projects/<project-id>/logs/GCEMetadataScripts" OR "projects/<project-id>/logs/google_metadata_script_runner") AND resource.labels.instance_id=<id>' --format="table(timestamp, resource.labels.instance_id, jsonPayload.message)" --order=asc
```

**IMPORTANT:** If `build.sh` fails it will not clean up supporting
infrastructure (VPC), please perform following:

```sh
gcluster destroy /tmp/build_a3m-slurm-image/roll --auto-approve
gcluster destroy /tmp/build_common-slurm-image/roll --auto-approve
```
