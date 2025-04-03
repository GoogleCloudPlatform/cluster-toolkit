## Description

This module creates a Slinky cluster and nodeset(s), for a Slurm-on-Kubernetes HPC setup.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.16 |
| <a name="requirement_helm"></a> [helm](#requirement\_helm) | ~> 2.17 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.16 |
| <a name="provider_helm"></a> [helm](#provider\_helm) | ~> 2.17 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [helm_release.cert_manager](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.prometheus](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.slurm](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.slurm_operator](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_cert_manager_values"></a> [cert\_manager\_values](#input\_cert\_manager\_values) | Value overrides for the Cert Manager release | `any` | <pre>{<br/>  "crds": {<br/>    "enabled": true<br/>  }<br/>}</pre> | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | An identifier for the GKE cluster resource with format projects/<project\_id>/locations/<region>/clusters/<name>. | `string` | n/a | yes |
| <a name="input_install_kube_prometheus_stack"></a> [install\_kube\_prometheus\_stack](#input\_install\_kube\_prometheus\_stack) | Install the Kube Prometheus Stack. | `bool` | `false` | no |
| <a name="input_node_pool_names"></a> [node\_pool\_names](#input\_node\_pool\_names) | Names of node pools, for use in node affinities (Slinky system components). | `list(string)` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID that hosts the GKE cluster. | `string` | n/a | yes |
| <a name="input_prometheus_values"></a> [prometheus\_values](#input\_prometheus\_values) | Value overrides for the Prometheus release | `any` | <pre>{<br/>  "installCRDs": true<br/>}</pre> | no |
| <a name="input_slurm_operator_values"></a> [slurm\_operator\_values](#input\_slurm\_operator\_values) | Value overrides for the Slinky release | `any` | `{}` | no |
| <a name="input_slurm_values"></a> [slurm\_values](#input\_slurm\_values) | Value overrides for the Slurm release | `any` | `{}` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Post deployment instructions. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
