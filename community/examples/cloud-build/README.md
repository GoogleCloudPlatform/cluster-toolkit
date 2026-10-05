# Remote Container Image Build with Cloud Build

The [cloud-build-remote.yaml](cloud-build-remote.yaml) blueprint demonstrates how to use the [`artifact-registry`](../../modules/container/artifact-registry/README.md) and [`cloud-build`](../../modules/container/cloud-build/README.md) community modules to create a Docker repository in Google Cloud Artifact Registry and build a container image directly from a remote GitHub repository using Google Cloud Build.

In this example, the blueprint:
1. Provisions a standard Docker repository in Artifact Registry (`artifact-repository`).
2. Clones the [AlphaFold 3](https://github.com/google-deepmind/alphafold3) GitHub repository at tag `v3.0.4` inside a Cloud Build worker (`E2_HIGHCPU_32`) using the [build-alphafold3.yaml.tfpl](build-alphafold3.yaml.tfpl) template.
3. Builds and pushes the `alphafold3:v3.0.4` container image to the newly created Artifact Registry repository, skipping the build if that versioned tag already exists (`skip_if_exists`).

## Prerequisites

Ensure the following Google Cloud APIs are enabled in your project:

- `artifactregistry.googleapis.com` (Artifact Registry API)
- `cloudbuild.googleapis.com` (Cloud Build API)

```bash
gcloud services enable artifactregistry.googleapis.com cloudbuild.googleapis.com \
  --project=<PROJECT_ID>
```

## Deploy the Blueprint

From the root of the Cluster Toolkit repository, run `gcluster deploy`:

```bash
./gcluster deploy community/examples/cloud-build/cloud-build-remote.yaml \
  --vars project_id=<PROJECT_ID>
```

You can also override default variables such as `region`, `deployment_name`, or `af3_version`:

```bash
./gcluster deploy community/examples/cloud-build/cloud-build-remote.yaml \
  --vars project_id=<PROJECT_ID> \
  --vars region=us-central1 \
  --vars deployment_name=af3-msa \
  --vars af3_version=v3.0.4
```

## Verify the Built Image

Once deployment completes, list the container images in the Artifact Registry repository:

```bash
gcloud artifacts docker images list \
  us-central1-docker.pkg.dev/<PROJECT_ID>/af3-msa-artifact-repository/alphafold3 \
  --include-tags
```

## Clean Up

To destroy the deployed resources, run:

```bash
./gcluster destroy af3-msa
```
