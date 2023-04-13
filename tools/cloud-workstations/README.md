# Configuring Cloud Workstations for usage with the Cloud HPC Toolkit

> **_NOTE:_** If you want to redirect container registry repos to artifact registry, please see
> [this artifact registry guide](https://cloud.google.com/artifact-registry/docs/transition/setup-gcr-repo?&_ga=2.33584865.-1391632029.1681343137#redirect-enable).

## Create an artifact registry repository

The following will create a repository called `hpc-toolkit-workstation-image` in gcloud's default cloud project.

```sh
PROJECT_ID=$(gcloud config get project)
LOCATION=us
REPO=hpc-toolkit-workstation-image

gcloud artifacts repositories create ${REPO} --repository-format=docker --location=${LOCATION} --project=${PROJECT_ID}
```

## Build a Cloud Workstation container with all developer dependencies for the HPC Toolkit

To build the Cloud workstation container as defined in the [Dockerfile](./Dockerfile), run the following command from the root of the HPC-Toolkit repo:

```sh
gcloud builds submit --config=tools/cloud-workstations/workstation-image.yaml --substitutions _LOCATION=${LOCATION},_REPO=${REPO} --project ${PROJECT_ID}
```

## Create the Cloud Workstations Cluster and configuration

Create a Google Cloud Workstations by following the instructions in https://cloud.google.com/workstations/docs/create-workstation.
Make sure that during the "Create Configuration" phase, you click on the `Environment Configuration` and choose the custom container image we built above by clicking in the `SELECT` button.
