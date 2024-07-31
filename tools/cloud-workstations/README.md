# Configuring Cloud Workstations for usage with the Cluster Toolkit

> **_NOTE:_** If you want to redirect container registry repos to artifact registry, please see
> [this artifact registry guide](https://cloud.google.com/artifact-registry/docs/transition/setup-gcr-repo?&_ga=2.33584865.-1391632029.1681343137#redirect-enable).

## Create an artifact registry repository

Set the variables to be used in the commands below.

> **_NOTE:_** Replace the REGION value with a region that you want to host your workstations in.  The CUSTOM_IMAGE location won't exist until the image is created, but it can still be set in advance.

```sh
PROJECT_ID=$(gcloud config get project)
LOCATION=us
REGION=<your region, e.g. us-central-1>
WORKSTATION_NAME=hpc-toolkit-workstation
CLUSTER_NAME=$WORKSTATION_NAME-cluster
CONFIG_NAME=$WORKSTATION_NAME-config
CUSTOM_IMAGE=us-docker.pkg.dev/${PROJECT_ID}/${WORKSTATION_NAME}/hpc-toolkit-workstation:latest
MACHINE_TYPE=e2-standard-8
SERVICE_ACCOUNT=<service account email>
```

The following will create a repository called `hpc-toolkit-workstation-image` in gcloud's default cloud project.

```sh
gcloud artifacts repositories create ${WORKSTATION_NAME} --repository-format=docker --location=${LOCATION} --project=${PROJECT_ID}
```

## Build a Cloud Workstation container with all developer dependencies for the Cluster Toolkit

To build the Cloud workstation container as defined in the [Dockerfile](./Dockerfile), run the following command from the root of the Cluster-Toolkit repo:

```sh
gcloud builds submit --config=tools/cloud-workstations/workstation-image.yaml --substitutions _LOCATION=${LOCATION},_REPO=${WORKSTATION_NAME} --project ${PROJECT_ID}
```

## Create the Cloud Workstations cluster and configuration

Create a Google Cloud Workstations by following the instructions in https://cloud.google.com/workstations/docs/create-workstation.
Make sure that during the "Create Configuration" phase, you click on the `Environment Configuration` and choose the custom container image we built above by clicking in the `SELECT` button.

The examples below are cloud shell (`gcloud`) commands that utilize the environment variable set at the top of this document.

### Example creation of a Cloud Workstation cluster

```sh
gcloud workstations clusters create ${CLUSTER_NAME} --region=${REGION} --project=${PROJECT_ID}
```

> **_NOTE:_** If the workstation won't start and gives an error about the cluster being deprecated, you may need to enter the cloud console and update the service account to the default.

### Example creation of a Cloud Workstation configuration

This uses the latest docker image from the instructions above.  If a different image is required, please replace the `--container-custom-image` with the correct image and hash/tag.

> **_NOTE:_** Users should determine the service account to use with the command `gcloud iam service-accounts list`.

```sh
gcloud workstations configs create ${CONFIG_NAME} --cluster=${CLUSTER_NAME} --region=${REGION} --project=${PROJECT_ID} --machine-type=${MACHINE_TYPE} --container-custom-image=${CUSTOM_IMAGE} --service-account=${SERVICE_ACCOUNT}
```

## Create the Cloud Workstation

Once the Cloud Workstations cluster and configuration are built, workstations can be built.

### Example creation of Cloud Workstation

```sh
gcloud workstations create ${WORKSTATION_NAME} --cluster=${CLUSTER_NAME} --config=${CONFIG_NAME} --region=${REGION} 
```

Once this is complete, the cloud console can be used to start and launch the workstation.

## Using the Cloud Workstation

> [!IMPORTANT]
> When the workstation is launched for the first time, the system will clone cluster-toolkit and install a number of useful code-oss extensions for working with the toolkit.  This will be done in the background.  Please allow 4-5 min for installation.

Once built and upon initial launch (assuming no changes were made to the files used to build the workstation image), the workstation should have a clean version of the main branch Cluster Toolkit in the user's home directory, as well as all of the prerequisites required to build and run pre-commit (see [Development](../../README.md#development)).

The final setup steps are:

* Updating up your git settings
  * User name and email
  * SSH keys for Github
* Cloning a forked repo
  * Run `pre-commit install` in each new cloned repository to make sure that pre-commit is run during each commit
