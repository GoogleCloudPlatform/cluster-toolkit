# Colab Enterprise Module

Provisions a Vertex AI Colab Enterprise runtime template (`google_colab_runtime_template`) and attached runtime instance (`google_colab_runtime`) using official Google Cloud Terraform provider resources.

---

## Features

- **Automatic GCS Bucket Mounting**: Pass `mount_gcs_bucket = "my-bucket"` to automatically generate and upload a clean `gcsfuse` mounting script to GCS and attach it to the VM startup sequence. Mounted locally at `/home/jupyter/gcs` (or custom `mount_path`).
- **Flexible Hardware Options**: Configurable standard CPU machine types or GPU acceleration (NVIDIA L4, T4, etc.).

---

## Example Usage

### Standard Colab Enterprise Runtime with GCS Mount

```yaml
- group: colab
  modules:
  - id: agent-platform-colab
    source: community/modules/agent-platform/colab
    settings:
      project_id: my-project-id
      region: us-central1
      deployment_name: my-deployment
      mount_gcs_bucket: my-data-bucket
      mount_path: /home/jupyter/gcs
      colab_machine_type: n2-standard-4
      environment_variables:
        BUCKET_NAME: my-data-bucket
        PROJECT_ID: my-project-id
```

### Advanced Usage with GPUs & Custom Persistent Disk

```yaml
- group: colab
  modules:
  - id: agent-platform-colab
    source: community/modules/agent-platform/colab
    settings:
      project_id: my-project-id
      region: us-central1
      deployment_name: my-deployment
      mount_gcs_bucket: my-data-bucket
      colab_machine_type: g2-standard-4
      accelerator_type: NVIDIA_L4
      accelerator_count: 1
      disk_type: pd-balanced
      disk_size_gb: 20
      idle_timeout: 14400s
      environment_variables:
        BUCKET_NAME: my-data-bucket
        PROJECT_ID: my-project-id
```

---

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.2.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 5.20.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | >= 3.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 5.20.0 |
| <a name="provider_random"></a> [random](#provider\_random) | >= 3.0.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_colab_runtime.runtime](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/colab_runtime) | resource |
| [google_colab_runtime_template.template](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/colab_runtime_template) | resource |
| [google_storage_bucket_object.mount_script](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [random_id.suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [google_client_openid_userinfo.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_openid_userinfo) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_accelerator_count"></a> [accelerator\_count](#input\_accelerator\_count) | Optional number of GPU accelerators to attach to the VM. | `number` | `null` | no |
| <a name="input_accelerator_type"></a> [accelerator\_type](#input\_accelerator\_type) | Optional GPU accelerator type (e.g. 'NVIDIA\_TESLA\_T4', 'NVIDIA\_L4'). | `string` | `null` | no |
| <a name="input_colab_machine_type"></a> [colab\_machine\_type](#input\_colab\_machine\_type) | Machine type for the Colab runtime VM (e.g., 'n2-standard-4'). | `string` | `"n2-standard-4"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Deployment name used as prefix for runtime template and runtime IDs. | `string` | n/a | yes |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of the user persistent disk in GB. | `number` | `20` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Persistent disk type for the Colab runtime VM ('pd-standard', 'pd-ssd', 'pd-balanced'). | `string` | `"pd-balanced"` | no |
| <a name="input_enable_internet_access"></a> [enable\_internet\_access](#input\_enable\_internet\_access) | Enable internet access for the Colab runtime VM. | `bool` | `true` | no |
| <a name="input_environment_variables"></a> [environment\_variables](#input\_environment\_variables) | Map of environment variables to inject into the Colab runtime environment. | `map(string)` | `{}` | no |
| <a name="input_idle_timeout"></a> [idle\_timeout](#input\_idle\_timeout) | Optional idle timeout duration for automatic shutdown (e.g. '14400s' for 4 hours). | `string` | `null` | no |
| <a name="input_module_instance_id"></a> [module\_instance\_id](#input\_module\_instance\_id) | Unique ID of this module instance (automatically populated by gcluster). | `string` | `"agent-platform-colab"` | no |
| <a name="input_mount_gcs_bucket"></a> [mount\_gcs\_bucket](#input\_mount\_gcs\_bucket) | Optional GCS bucket name to automatically mount to the VM via gcsfuse on startup. | `string` | `null` | no |
| <a name="input_mount_path"></a> [mount\_path](#input\_mount\_path) | Local VM directory path where mount\_gcs\_bucket will be mounted. | `string` | `"/home/jupyter/gcs"` | no |
| <a name="input_network"></a> [network](#input\_network) | Optional VPC network resource name or ID for private networking. | `string` | `null` | no |
| <a name="input_post_startup_script_behavior"></a> [post\_startup\_script\_behavior](#input\_post\_startup\_script\_behavior) | Execution behavior for post\_startup\_script ('RUN\_ONCE', 'RUN\_EVERY\_START', 'DOWNLOAD\_AND\_RUN\_EVERY\_START'). | `string` | `"DOWNLOAD_AND_RUN_EVERY_START"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | GCP region where the Colab runtime and template will be provisioned. | `string` | n/a | yes |
| <a name="input_runtime_user"></a> [runtime\_user](#input\_runtime\_user) | The user email for the Colab runtime. Defaults to active gcloud/Terraform user if not set. | `string` | `null` | no |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | Optional VPC subnet resource name or ID for private networking. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_id"></a> [id](#output\_id) | The unique resource name of the Colab runtime template. |
| <a name="output_runtime_id"></a> [runtime\_id](#output\_runtime\_id) | The generated ID of the Colab runtime. |
| <a name="output_template_id"></a> [template\_id](#output\_template\_id) | The generated ID of the Colab runtime template. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
