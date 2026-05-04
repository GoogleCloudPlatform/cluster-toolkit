## Description
This module fetches the BackendService associated with a GKE Ingress service. It uses a Python script to query the GKE cluster and return the backend service name and ID.

### Software Requirements

* Python 3 (with `json` support)
* `gcloud` CLI
* `kubectl` CLI

### Permissions

* Active authentication with permissions to view GKE clusters and Compute backend services.

### Example

```yaml
  - id: fetch_http_backend
    source: community/modules/scripts/gke-backend-fetcher
    settings:
      project_id: $(vars.project_id)
      cluster_name: my-cluster
      location: us-central1
      namespace: default
      service_name: http-service
      service_port: "8080"
```

## License
<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_external"></a> [external](#requirement\_external) | >= 2.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_external"></a> [external](#provider\_external) | >= 2.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [external_external.backend_fetcher](https://registry.terraform.io/providers/hashicorp/external/latest/docs/data-sources/external) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Name of the GKE cluster. | `string` | n/a | yes |
| <a name="input_location"></a> [location](#input\_location) | Location (zone or region) of the GKE cluster. | `string` | n/a | yes |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | Kubernetes namespace where the Ingress is deployed. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID where the GKE cluster and Ingress reside. | `string` | n/a | yes |
| <a name="input_service_name"></a> [service\_name](#input\_service\_name) | Name of the Kubernetes Service the BackendService is backing (e.g. 'http-service'). | `string` | n/a | yes |
| <a name="input_service_port"></a> [service\_port](#input\_service\_port) | Port of the Kubernetes Service (e.g. '8080'). | `string` | n/a | yes |
| <a name="input_timeout_seconds"></a> [timeout\_seconds](#input\_timeout\_seconds) | Maximum time to wait for the backend service to be provisioned (in seconds). | `number` | `600` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_backend_service_id"></a> [backend\_service\_id](#output\_backend\_service\_id) | The dynamically assigned ID of the Google Cloud BackendService. |
| <a name="output_backend_service_name"></a> [backend\_service\_name](#output\_backend\_service\_name) | The dynamically assigned name of the Google Cloud BackendService. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
