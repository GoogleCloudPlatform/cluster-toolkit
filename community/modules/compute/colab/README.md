## Description

Provisions a Vertex AI Colab Enterprise runtime template (`google_colab_runtime_template`)
and attached runtime instance (`google_colab_runtime`) using official Google Cloud
Terraform provider resources.

### Features

- **Automatic GCS Bucket Mounting**: Pass `mount_gcs_bucket = "my-bucket"` to
  automatically generate and upload a clean `gcsfuse` mounting script to GCS and
  attach it to the VM startup sequence. Mounted locally at `/home/jupyter/gcs` (or custom `mount_path`).
- **Flexible Hardware Options**: Configurable standard CPU machine types or GPU
  acceleration (NVIDIA L4, T4, etc.).

### Example

The following example creates a standard Colab Enterprise runtime with automatic GCS mounting:

```yaml
- group: colab
  modules:
  - id: agent-platform-colab
    source: community/modules/compute/colab
    settings:
      project_id: $(vars.project_id)
      region: $(vars.region)
      deployment_name: $(vars.deployment_name)
      runtime_user: user@example.com
      mount_gcs_bucket: my-data-bucket
      mount_path: /home/jupyter/gcs
      colab_machine_type: n2-standard-4
      environment_variables:
        BUCKET_NAME: my-data-bucket
        PROJECT_ID: $(vars.project_id)
```

The following example provisions a GPU-accelerated runtime with idle shutdown configured:

```yaml
- group: colab
  modules:
  - id: agent-platform-colab
    source: community/modules/compute/colab
    settings:
      project_id: $(vars.project_id)
      region: $(vars.region)
      deployment_name: $(vars.deployment_name)
      runtime_user: user@example.com
      mount_gcs_bucket: my-data-bucket
      colab_machine_type: g2-standard-4
      accelerator_type: NVIDIA_L4
      accelerator_count: 1
      disk_type: pd-balanced
      disk_size_gb: 20
      idle_timeout: 14400s
      environment_variables:
        BUCKET_NAME: my-data-bucket
        PROJECT_ID: $(vars.project_id)
```

## Day-2 operations

Most of this module's inputs map to fields that Vertex AI treats as immutable. Changing them
replaces the runtime template, and because the runtime references the template, the runtime is
replaced too — the runtime's data persistent disk is destroyed and recreated, and any files in
the notebook home directory are lost. Work stored in the bucket mounted via `mount_gcs_bucket`
survives, which is a good reason to use it.

Requires provider `>= 7.26.0`. Earlier versions have no update path for the runtime template at
all, so every change below becomes a replacement.

### Updated in place

| Input | Notes |
|---|---|
| `environment_variables` | |
| `post_startup_script_behavior` | |
| `mount_gcs_bucket` | Changes the startup script URL. |
| `labels` | |
| `idle_timeout` | Only when changing one duration to another — see below. |

### Replaces the runtime template and the runtime

| Input | Notes |
|---|---|
| `colab_machine_type` | |
| `accelerator_type`, `accelerator_count` | |
| `disk_type`, `disk_size_gb` | |
| `network`, `subnetwork`, `enable_internet_access` | |
| `runtime_user` | Replaces the runtime only; the template is untouched. |
| `deployment_name`, or the module's blueprint id | Both resource names are derived from these. |
| `idle_timeout` set to or from null | Adds or removes the `idle_shutdown_config` block, which is immutable even though the duration inside it is not. |

### Changes that produce no Terraform diff

Editing `mount_path` (or anything else that only alters the generated mount script's contents
rather than its GCS URL) updates the object in the bucket but produces no diff on the runtime
template. The new script takes effect the next time the runtime starts, and only when
`post_startup_script_behavior` is `DOWNLOAD_AND_RUN_EVERY_START`. With `RUN_ONCE` it will never
re-run on an existing runtime.

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.26.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.26.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_colab_runtime.runtime](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/colab_runtime) | resource |
| [google_colab_runtime_template.template](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/colab_runtime_template) | resource |
| [google_storage_bucket_object.mount_script](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_accelerator_count"></a> [accelerator\_count](#input\_accelerator\_count) | Optional number of GPU accelerators to attach to the VM. | `number` | `null` | no |
| <a name="input_accelerator_type"></a> [accelerator\_type](#input\_accelerator\_type) | Optional GPU accelerator type (e.g. 'NVIDIA\_TESLA\_T4', 'NVIDIA\_L4'). | `string` | `null` | no |
| <a name="input_colab_machine_type"></a> [colab\_machine\_type](#input\_colab\_machine\_type) | Machine type for the Colab runtime VM (e.g., 'n2-standard-4'). | `string` | `"n2-standard-4"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Deployment name used as prefix for runtime template and runtime IDs. | `string` | n/a | yes |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of the user persistent disk in GB. | `number` | `20` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Persistent disk type for the Colab runtime VM ('pd-standard', 'pd-balanced', 'pd-ssd', 'hyperdisk-balanced'). | `string` | `"pd-balanced"` | no |
| <a name="input_enable_internet_access"></a> [enable\_internet\_access](#input\_enable\_internet\_access) | Enable internet access for the Colab runtime VM. | `bool` | `true` | no |
| <a name="input_environment_variables"></a> [environment\_variables](#input\_environment\_variables) | Map of environment variables to inject into the Colab runtime environment. | `map(string)` | `{}` | no |
| <a name="input_idle_timeout"></a> [idle\_timeout](#input\_idle\_timeout) | Optional idle timeout duration after which the runtime is automatically shut down (e.g. '14400s' for 4 hours). Valid range is [10m, 24h] (600s to 86400s). An input of '0s' disables the idle shutdown feature. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to assign to the Colab runtime template. | `map(string)` | `{}` | no |
| <a name="input_module_instance_id"></a> [module\_instance\_id](#input\_module\_instance\_id) | Unique ID of this module instance (automatically populated by gcluster). | `string` | `"colab"` | no |
| <a name="input_mount_gcs_bucket"></a> [mount\_gcs\_bucket](#input\_mount\_gcs\_bucket) | Optional GCS bucket name to automatically mount to the VM via gcsfuse on startup. | `string` | `null` | no |
| <a name="input_mount_path"></a> [mount\_path](#input\_mount\_path) | Local VM directory path where mount\_gcs\_bucket will be mounted. | `string` | `"/home/jupyter/gcs"` | no |
| <a name="input_network"></a> [network](#input\_network) | Optional VPC network resource name or ID for private networking. | `string` | `null` | no |
| <a name="input_post_startup_script_behavior"></a> [post\_startup\_script\_behavior](#input\_post\_startup\_script\_behavior) | Execution behavior for post\_startup\_script ('RUN\_ONCE', 'RUN\_EVERY\_START', 'DOWNLOAD\_AND\_RUN\_EVERY\_START'). | `string` | `"DOWNLOAD_AND_RUN_EVERY_START"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | GCP region where the Colab runtime and template will be provisioned. | `string` | n/a | yes |
| <a name="input_runtime_user"></a> [runtime\_user](#input\_runtime\_user) | The user email for the Colab runtime (required by Colab Enterprise, e.g. 'user@example.com'). | `string` | n/a | yes |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | Optional VPC subnet resource name or ID for private networking. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_id"></a> [id](#output\_id) | The unique resource name of the Colab runtime template. |
| <a name="output_runtime_name"></a> [runtime\_name](#output\_runtime\_name) | The name of the Colab runtime. |
| <a name="output_template_name"></a> [template\_name](#output\_template\_name) | The name of the Colab runtime template. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
