# Description

This module creates:

- **A local file:** A Google Cloud Batch job template file is created. See the
  `instructions` output for the location of the file and instructions on how to
  submit it to Batch.
- **An [instance template]:** This instance template defines the compute settings to
  be used for the Batch job such as network, machine type, image, and startup
  script. This instance template is automatically referenced from the Batch job
  template described above.

[instance template]: https://cloud.google.com/compute/docs/instance-templates

When this module is used with the `cloud-batch-login-node` module, the generated
job template will be placed on the login node.

In some cases the job template can be submitted to the Google Cloud Batch API
without modification, but for more complex workloads it is expected that the
user will modify the template after running the HPC Toolkit.

## Example

```yaml
- id: batch-job
  source: community/modules/scheduler/cloud-batch-job
  use: [network1]
  settings:
    runnable: "echo 'hello world'"
    machine_type: n2-standard-4
  outputs: [instructions]
```

See the
[Google Cloud Batch Example](../../../../examples/README.md#cloud-batchyaml--)
for how to use the `cloud-batch-job` module with other HPC Toolkit modules such
as `filestore` and `startup-script`.

## Shared VPC

This module supports using a [shared VPC] with a Batch job. To accomplish this,
include a [`pre-existing-vpc`] module that references an existing shared VPC and
then have the `cloud-batch-job` module `use` the `pre-existing-vpc`.

[shared VPC]: https://cloud.google.com/vpc/docs/shared-vpc
[`pre-existing-vpc`]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/network/pre-existing-vpc

## Instance Templates

Many of the settings for a Google Cloud Batch job are set using an instance
template, `machine_type` for example. The `cloud-batch-job` module accomplishes
this by creating an instance template within the module, which is supplied to
the Google Cloud Batch job.

Alternatively, one can supply an instance template to the `cloud-batch-job`
module using the `instance_template` setting. This supplied instance template
could be generated outside of the HPC Toolkit (via the Cloud Console UI for
example) or using a separate module within the blueprint. To define an instance
template within a blueprint, one can use the Cloud Foundation Toolkit instance
template module as shown in the following example. This can be useful when
trying to set a property not natively supported in the `cloud-batch-job` module.

### Example generating instance template using Cloud Foundation Toolkit module

```yaml
deployment_groups:
- group: primary
  modules:
  - id: network1
    source: modules/network/pre-existing-vpc

  - id: appfs
    source: modules/file-system/filestore
    use: [network1]

  - id: batch-startup-script
    source: modules/scripts/startup-script
    settings:
      runners: ...
          
  - id: batch-compute-template
    source: github.com/terraform-google-modules/terraform-google-vm//modules/instance_template?ref=v7.8.0
    use: [batch-startup-script]
    settings:
      # Boiler plate to work with Cloud Foundation Toolkit
      network: $(network1.network_self_link)
      service_account: {email: null, scopes: ["https://www.googleapis.com/auth/cloud-platform"]}
      access_config: [{nat_ip: null, network_tier: null}]
      # Batch customization
      machine_type: n2-standard-4
      metadata:
        network_storage: ((jsonencode([module.appfs.network_storage])))
      source_image_family: hpc-centos-7
      source_image_project: cloud-hpc-image-public

  - id: batch-job
    source: ./community/modules/scheduler/cloud-batch-job
    settings:
      instance_template: $(batch-compute-template.self_link)
    outputs: [instructions]
```

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2022 Google LLC

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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13.0 |
| <a name="requirement_local"></a> [local](#requirement\_local) | >= 2.0.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_local"></a> [local](#provider\_local) | >= 2.0.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_instance_template"></a> [instance\_template](#module\_instance\_template) | terraform-google-modules/vm/google//modules/instance_template | > 7.6.0 |

## Resources

| Name | Type |
|------|------|
| [local_file.job_template](https://registry.terraform.io/providers/hashicorp/local/latest/docs/resources/file) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, used for the job\_id | `string` | n/a | yes |
| <a name="input_gcloud_version"></a> [gcloud\_version](#input\_gcloud\_version) | The version of the gcloud cli being used. Used for output instructions. Valid inputs are `"alpha"`, `"beta"` and "" (empty string for default version) | `string` | `"alpha"` | no |
| <a name="input_image"></a> [image](#input\_image) | Google Cloud Batch compute node image. Ignored if `instance_template` is provided. | <pre>object({<br>    family  = string,<br>    project = string<br>  })</pre> | <pre>{<br>  "family": "hpc-centos-7",<br>  "project": "cloud-hpc-image-public"<br>}</pre> | no |
| <a name="input_instance_template"></a> [instance\_template](#input\_instance\_template) | Compute VM instance template self-link to be used for Google Cloud Batch compute node. If provided, a number of other variables will be ignored as noted by `Ignored if instance_template is provided` in descriptions. | `string` | `null` | no |
| <a name="input_job_filename"></a> [job\_filename](#input\_job\_filename) | The filename of the generated job template file. Will default to `cloud-batch-<job_id>.json` if not specified | `string` | `null` | no |
| <a name="input_job_id"></a> [job\_id](#input\_job\_id) | An id for the Google Cloud Batch job. Used for output instructions and file naming. Defaults to deployment name. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the Google Cloud Batch compute nodes. List key, value pairs. Ignored if `instance_template` is provided. | `any` | n/a | yes |
| <a name="input_log_policy"></a> [log\_policy](#input\_log\_policy) | Create a block to define log policy.<br>When set to `CLOUD_LOGGING`, logs will be sent to Cloud Logging.<br>When set to `PATH`, path must be added to generated template.<br>When set to `DESTINATION_UNSPECIFIED`, logs will not be preserved. | `string` | `"CLOUD_LOGGING"` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for Google Cloud Batch compute nodes. Ignored if `instance_template` is provided. | `string` | `"n2-standard-4"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. Ignored if `instance_template` is provided. | <pre>list(object({<br>    server_ip     = string,<br>    remote_mount  = string,<br>    local_mount   = string,<br>    fs_type       = string,<br>    mount_options = string<br>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region in which to run the Google Cloud Batch job | `string` | n/a | yes |
| <a name="input_runnable"></a> [runnable](#input\_runnable) | A string to be executed as the main workload of the Google Cloud Batch job. This will be used to populate the generated template. | `string` | `"## Add your workload here"` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the Google Cloud Batch compute node. Ignored if `instance_template` is provided. | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/devstorage.read_only",<br>    "https://www.googleapis.com/auth/logging.write",<br>    "https://www.googleapis.com/auth/monitoring.write",<br>    "https://www.googleapis.com/auth/servicecontrol",<br>    "https://www.googleapis.com/auth/service.management.readonly",<br>    "https://www.googleapis.com/auth/trace.append"<br>  ]<br>}</pre> | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script run before Google Cloud Batch job starts. Ignored if `instance_template` is provided. | `string` | `null` | no |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | The subnetwork that the Batch job should run on. Defaults to 'default' subnet. Ignored if `instance_template` is provided. | `any` | `null` | no |
| <a name="input_task_count"></a> [task\_count](#input\_task\_count) | Number of parallel tasks | `number` | `1` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_gcloud_version"></a> [gcloud\_version](#output\_gcloud\_version) | The version of gcloud to be used. |
| <a name="output_instance_template"></a> [instance\_template](#output\_instance\_template) | Instance template used by the Google Cloud Batch job. |
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions for submitting Google Cloud Batch job. |
| <a name="output_job_filename"></a> [job\_filename](#output\_job\_filename) | The filename of the generated Google Cloud Batch job template. |
| <a name="output_job_id"></a> [job\_id](#output\_job\_id) | The Google Cloud Batch job id. |
| <a name="output_job_template_contents"></a> [job\_template\_contents](#output\_job\_template\_contents) | The generated Google Cloud Batch job template. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
