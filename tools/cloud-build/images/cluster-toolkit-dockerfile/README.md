# Cluster Toolkit Dockerfile

This repository contains a Dockerfile for building a Docker image with Cluster Toolkit and its dependencies installed and the `gcluster` binary readily available for use.

## System Requirements

* [Docker Engine](https://docs.docker.com/engine/) needs to be installed.

## Environment Variables
The following environment variables can be used to customize the build process:
* **`BASE_IMAGE`**: The base image to use for the build. Defaults to `gcr.io/google.com/cloudsdktool/google-cloud-cli:stable`.
* **`TERRAFORM_VERSION`**: The version of Terraform to install. Defaults to `1.5.2`.
* **`PACKER_VERSION`**: The version of Packer to install. Defaults to `1.8.6`.
* **`GO_VERSION`**: The version of Go to install. Defaults to `1.21.0`.
* **`CLUSTER_TOOLKIT_VERSION`**: The version (tag) of the Cluster Toolkit to install. Defaults to the current version associated with the `main` branch of the cluster toolkit repository.

## Build the Docker Image
To build the Docker image, navigate to the directory the Dockerfile is present in and run the following command:

```bash
docker build --build-arg BASE_IMAGE=<base_image> \
             --build-arg TERRAFORM_VERSION=<terraform_version> \
             --build-arg PACKER_VERSION=<packer_version> \
             --build-arg GO_VERSION=<go_version> \
             --build-arg CLUSTER_TOOLKIT_VERSION=<cluster_toolkit_version> \
             -t <image_name> .
```

Example:

```bash
docker build --build-arg CLUSTER_TOOLKIT_VERSION=v1.40.0 -t my-cluster-toolkit-image .
```

The above example builds an image tagged `my-cluster-toolkit-image` and sets the CLUSTER_TOOLKIT_VERSION to v1.40.0 while using the default values for other arguments.

## Run the Docker Image
To run the Docker image, use the following command:

```bash
docker run <image_name> <gcluster_command>
```

Replace the following placeholders:

* `<image_name>`: The name of your Docker image.
* `<gcluster_command>`: The gcluster command you want to execute.

Because this Dockerfile has an `ENTRYPOINT ["gcluster"]` line, any arguments provided to the docker run command after the `<image_name>` will be passed as arguments to the `gcluster` command within the container.

Example:

```bash
docker run my-cluster-toolkit-image --version
```

This command will execute `gcluster --version` within the container. You can use this pattern to run any valid gcluster command within the container.

## Sharing data between local and container environments
To use gcluster commands that interact with Google Cloud, you need to provide your Google Cloud credentials to the container. You can do this by mounting your gcloud configuration directory:

```bash
docker run -v ~/.config/gcloud/:/root/.config/gcloud <image_name> <gcluster_command>
```

This mounts your local `~/.config/gcloud` directory to the `/root/.config/gcloud` directory inside the container, allowing the gcluster binary to access your credentials.

To pass in a blueprint stored locally to the `glcuster` binary inside of the container, you can also mount the directory containing the blueprint to the container:

```bash
docker run -v ~/.config/gcloud/:/root/.config/gcloud -v $(pwd):/data <image_name> <gcluster_command>
```

This mounts your current working directory ($(pwd)) to the `/data` directory inside the container. You can then reference the blueprint file within your <gcluster_command> using the /data path.

When using the docker container to run gcluster commands that either deploy or modify cloud resources, it's strongly recommended to save a local copy of the deployment folder that's generated inside of the docker container. This can be done by using the `--out` flag in combination with some `gcluster` sub-commands to set the output directory where the gcluster deployment directory will be created. Ideally, the output directory should be a directory mounted from your local machine to the docker container. This ensures that the deployment folder persists even after the container exits.

Here's an example of how to use the `--out` flag to save the deployment folder to your current directory (mounted as `/data` in the container) when the `deploy` sub-command is used:

```bash
docker run -v ~/.config/gcloud/:/root/.config/gcloud -v $(pwd):/data my-cluster-toolkit-image deploy /data/my-blueprint.yaml --out /data --auto-approve
```

In this example, the deployment folder will be created in your current directory on your local machine, allowing you to access and manage the deployment artifacts even after the container is removed. The `--auto-approve` flag automatically approves any prompts from gcluster, streamlining the deployment process.
