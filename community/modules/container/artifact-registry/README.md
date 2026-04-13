## Description

This module provides ways to create and manage Google Cloud Artifact Registry repositories.

Currently this module is built to support repositories in Docker format although there are placeholder variables for other types which may work too. Remote repositories with pull-through cache functionality integrated with Google Secret Manager is currently supported. The aim of this module is to eventually offer feature parity with this [Terraform module](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/artifact_registry_repository#nested_remote_repository_config), allowing creation of repositories in various formats, including Docker, Maven, NPM, Python, APT, YUM, and COMMON.

This module is best suited for managing artifact repositories in HPC/AI containerized environments where artifacts need to be shared across distributed systems. It includes IAM role configurations and secret access handling for seamless integration with CI/CD pipelines and other services too.

It is designed to help facilitate containerized workloads running in the Cluster Toolkit with SLURM leveraging [Enroot](https://github.com/NVIDIA/enroot) and [Pyxis](https://github.com/NVIDIA/pyxis). Docker repositories can store container images that are used in job submissions, enabling efficient and scalable execution of containerized HPC or AI based workloads.

## Usage

### Service Account / APIs

You will need to enable the relevant APIs and create a Service Account for your cluster with the following Artifact Registry permissions.

```yaml
  - id: services-api
    source: community/modules/project/service-enablement
    settings:
      gcp_service_list:
        - secretmanager.googleapis.com
        - cloudbuild.googleapis.com
        - artifactregistry.googleapis.com

  - source: modules/project/service-account
    kind: terraform
    id: hpc_service_account
    settings:
      project_id: project_name
      name: service_account_name
      project_roles:
      - artifactregistry.reader
      - artifactregistry.writer
      - secretmanager.secretAccessor
```

### Deployment

Create a standard Docker repository.

```yaml
- id: registry
  source: community/modules/container/artifact-registry
  settings:
    repo_mode: STANDARD_REPOSITORY
    format: DOCKER
```

Mirror of public Docker Hub repository.

```yaml
- id: dockerhub_registry
  source: community/modules/container/artifact-registry
  settings:
    repo_mode: REMOTE_REPOSITORY
    format: DOCKER
    repo_public_repository: DOCKER_HUB
```

Mirror of NVIDIA's [NGC Catalog](https://catalog.ngc.nvidia.com/containers). [API key](https://org.ngc.nvidia.com/setup/api-key) used in blueprint is stored in Secret Manager.

```yaml
- id: ngc_registry
  source: community/modules/container/artifact-registry
  settings:
    repo_mode: REMOTE_REPOSITORY
    format: DOCKER
    repo_mirror_url: "https://nvcr.io"
    repo_username: $oauthtoken
    repo_password: api_key_here
    use_upstream_credentials: True
```

### Container Operations

Retrieve `$REPOSITORY_NAME` from [Artifact Registry](https://console.cloud.google.com/artifacts) or by using `gcloud`.

```yaml
gcloud artifacts repositories list --project="${PROJECT_ID}"
```

Pulling containers from your mirrored internal Artifact Repositories.

Pull [Ubuntu](https://hub.docker.com/_/ubuntu) from Docker Hub mirror.

```yaml
docker pull ${REGION}-docker.pkg.dev/${PROJECT_NAME}/${REPOSITORY_NAME}/library/ubuntu:latest
```

Pull [Pytorch](https://catalog.ngc.nvidia.com/orgs/nvidia/containers/pytorch) from NGC Catalog mirror.

```yaml
docker pull ${REGION}-docker.pkg.dev/${PROJECT_NAME}/${REPOSITORY_NAME}/nvidia/pytorch:24.11-py3
```

Alternatively, proceed with running SLURM's [NVIDIA/pyxis](https://github.com/NVIDIA/pyxis) plugin, which will now be able to pull and use these containers directly from the mirrored repositories.

Note: only Docker registries have been tested so far. Placeholders do exist for other registry types which may or may not work.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.42 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.42 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_artifact_registry_repository.artifact_registry](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/artifact_registry_repository) | resource |
| [google_secret_manager_secret.repo_password_secret](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret_version.repo_password_secret_version](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_version) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [random_password.repo_password](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/password) | resource |
| [terraform_data.input_validation](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment. | `string` | n/a | yes |
| <a name="input_format"></a> [format](#input\_format) | Artifact Registry format (e.g., DOCKER). | `string` | `"DOCKER"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the artifact registry. Key-value pairs. | `map(string)` | `{}` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID where the artifact registry and secret are created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region for the artifact registry. | `string` | n/a | yes |
| <a name="input_repo_mirror_url"></a> [repo\_mirror\_url](#input\_repo\_mirror\_url) | For REMOTE\_REPOSITORY, URL for a custom or common mirror. | `string` | `null` | no |
| <a name="input_repo_mode"></a> [repo\_mode](#input\_repo\_mode) | Artifact Registry mode (STANDARD\_REPOSITORY, REMOTE\_REPOSITORY, etc.). | `string` | `"STANDARD_REPOSITORY"` | no |
| <a name="input_repo_password"></a> [repo\_password](#input\_repo\_password) | Optional password/API key. If null, one will be randomly generated. | `string` | `null` | no |
| <a name="input_repo_public_repository"></a> [repo\_public\_repository](#input\_repo\_public\_repository) | For REMOTE\_REPOSITORY, name of a known public repo as per the Terraform module<br/>(e.g., DOCKER\_HUB) or null for custom repo. | `string` | `null` | no |
| <a name="input_repo_username"></a> [repo\_username](#input\_repo\_username) | Username for external repository. | `string` | `null` | no |
| <a name="input_repository_base"></a> [repository\_base](#input\_repository\_base) | For APT/YUM public repos, repository\_base (e.g., 'DEBIAN', 'UBUNTU'). | `string` | `null` | no |
| <a name="input_repository_path"></a> [repository\_path](#input\_repository\_path) | For APT/YUM public repos, repository\_path (e.g., 'debian/dists/buster'). | `string` | `null` | no |
| <a name="input_use_upstream_credentials"></a> [use\_upstream\_credentials](#input\_use\_upstream\_credentials) | Configure Service Account to use upstream credentials for REMOTE\_REPOSITORY:<br/>If true, a username/password is used for the REMOTE\_REPOSITORY mirror.<br/>If false (or if repo\_password == null), no password is created at all.<br/>Note: Blueprint credentials will be stored in Secrets Manager. | `bool` | `false` | no |
| <a name="input_user_managed_replication"></a> [user\_managed\_replication](#input\_user\_managed\_replication) | (Optional) A list of objects to enable user-managed replication.<br/>Each object can have:<br/>  location        = string<br/>  kms\_key\_name    = optional(string)<br/>If empty, auto replication is used. | <pre>list(object({<br/>    location     = string<br/>    kms_key_name = optional(string)<br/>  }))</pre> | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_registry_url"></a> [registry\_url](#output\_registry\_url) | The URL of the created artifact registry. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
