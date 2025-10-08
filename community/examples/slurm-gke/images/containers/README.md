# Slinky container image for GKE

This is a modified version of the original [Slinky Containers](https://github.com/SlinkyProject/slinky-containers) by SchedMD. This fork has been adapted for running on Google Kubernetes Engine(GKE).

## Prerequisites

Before you begin, ensure you have the following installed and configured:

* **Docker:** with the `buildx` plugin enabled.
* **Google Cloud SDK (`gcloud`):** for interacting with Google Artifact Registry.
* **Authentication:** You must be authenticated with Google Cloud and have configured Docker to use your credentials.

    ```bash
    # Authenticate with gcloud
    gcloud auth login

    # Configure Docker credentials for Artifact Registry (replace <LOCATION> with your region)
    gcloud auth configure-docker <LOCATION>-docker.pkg.dev
    ```

***

## 1. Setup Google Artifact Registry

You need a Docker repository in Google Artifact Registry to store your images.

1. **Choose a repository name** (e.g., `slurm-images`) and a **location** (e.g., `us-west1`).
2. Run the following `gcloud` command to create the repository:

   ```bash
   gcloud artifacts repositories create <REPO_NAME> \
       --repository-format=docker \
       --location=<LOCATION> \
       --description="Docker repository for Slurm images"
   ```

   *Replace `<REPO_NAME>` and `<LOCATION>` with your chosen values.*

***

## 2. Configure the Makefile

You **must** update the `Makefile` to point to your Artifact Registry repository.

1. Open the `Makefile` in a text editor.
2. Locate the `REGISTRY` and `REPO` variables.
3. Update them with your GCP project ID, location, and the repository name you just created.

   **Example:**
   If your project ID is `my-hpc-project`, your location is `us-west1`, and your repository name is `slurm-images`, the configuration should look like this:

   ```makefile
   # Container registry and repository
   REGISTRY ?= us-west1-docker.pkg.dev/my-hpc-project
   REPO ?= slurm-images
   ```

***

## 3. Build and Push the Images

The `Makefile` provides several targets to build and push the images individually or all at once.

### Build Images

* **Build both `slurmd` and `slurmd-pyxis` images:**

    ```bash
    make build
    ```

* **Build only the base `slurmd` image:**

    ```bash
    make build-slurmd
    ```

* **Build the `slurmd-pyxis` image** (this will also build the base `slurmd` image first as it's a dependency):

    ```bash
    make build-slurmd-pyxis
    ```

### Push Images

After building, you can push the images to your configured Artifact Registry.

* **Push both images:**

    ```bash
    make push
    ```

* **Push only the `slurmd` image:**

    ```bash
    make push-slurmd
    ```

* **Push only the `slurmd-pyxis` image:**

    ```bash
    make push-slurmd-pyxis
    ```

## License

This project is licensed under the Apache License, Version 2.0. The original copyright belongs to SchedMD LLC. My modifications are also licensed under the same terms.

A full copy of the license is available in the [LICENSE](./LICENSE) file.
