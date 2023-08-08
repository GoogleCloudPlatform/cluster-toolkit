# Configuring Cloud Workstations for usage with the Cloud HPC Toolkit

> **_NOTE:_** If you want to redirect container registry repos to artifact registry, please see
> [this artifact registry guide](https://cloud.google.com/artifact-registry/docs/transition/setup-gcr-repo?&_ga=2.33584865.-1391632029.1681343137#redirect-enable).

## Create an artifact registry repository

The following will create a repository called `hpc-toolkit-workstation-image` in gcloud's default cloud project.

```sh
PROJECT_ID=$(gcloud config get project)
LOCATION=us
REGION=us-central1
REPO=hpc-toolkit-workstation-image
PREFIX=hpc-toolkit-workstation

gcloud artifacts repositories create ${REPO} --repository-format=docker --location=${LOCATION} --project=${PROJECT_ID}
```

## Build a Cloud Workstation container with all developer dependencies for the HPC Toolkit

To build the Cloud workstation container as defined in the [Dockerfile](./Dockerfile), run the following command from the root of the HPC-Toolkit repo:

```sh
gcloud builds submit --config=tools/cloud-workstations/workstation-image.yaml --substitutions _LOCATION=${LOCATION},_REPO=${REPO} --project ${PROJECT_ID}
```

## Create the Cloud Workstations cluster and configuration

Create a Google Cloud Workstations by following the instructions in https://cloud.google.com/workstations/docs/create-workstation.
Make sure that during the "Create Configuration" phase, you click on the `Environment Configuration` and choose the custom container image we built above by clicking in the `SELECT` button.

The examples below are gcloud commands that utilize the environment variable set at the top of this document.  

### Example creation of a Cloud Workstation cluster:
```sh
CLUSTER_NAME=hpc-toolkit-workstation-cluster

gcloud workstations clusters create ${CLUSTER_NAME} --region=${REGION} --project=${PROJECT_ID}
```

### Example creation of a Cloud Workstation configuration:
This uses the latest docker image from the instruction above.  If a different image is required, please replace the `--container-custom-image` with the correct image and hash/tag.

```sh
CONFIG_NAME=hpc-toolkit-workstation-config

gcloud workstations configs create ${CONFIG_NAME} --cluster=${CLUSTER_NAME} --region=${REGION} --project=${PROJECT_ID} --machine-type=e2-standard-8 --container-custom-image=us-docker.pkg.dev/${PROJECT_ID}/${PREFIX}-image/hpc-toolkit-workstation:latest
```

### Example creation of Cloud Workstation:
```sh
WORKSTATION_NAME=hpc-toolkit-workstation

gcloud workstations create ${WORKSTATION_NAME} --cluster=${CLUSTER_NAME} --config=${CONFIG_NAME} --region=${REGION} 
```