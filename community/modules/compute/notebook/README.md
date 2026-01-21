# Description

This module creates the Vertex AI Notebook, to be used in tutorials.

Primarily used for FSI - MonteCarlo Tutorial: **[fsi-montecarlo-on-batch-tutorial]**.

[fsi-montecarlo-on-batch-tutorial]: ../docs/tutorials/fsi-montecarlo-on-batch/README.md

## Usage

This is a simple usage, using the default network:

```yaml
  - id: bucket
    source: modules/file-system/cloud-storage-bucket
    settings: 
      name_prefix: my-bucket
      local_mount: /home/jupyter/my-bucket

  - id: notebook
    source: community/modules/compute/notebook
    use: [bucket]
    settings:
      name_prefix: notebook
      machine_type: n1-standard-4

```

If the user wants do specify a custom subnetwork, or specific external IP restrictions, they can use the `network_interfaces` variable, here is an example on how to use a Shared VPC Subnet with an ephemeral external IP:

```yaml
  - id: bucket
    source: modules/file-system/cloud-storage-bucket
    settings: 
      name_prefix: my-bucket
      local_mount: /home/jupyter/my-bucket

  - id: notebook
    source: community/modules/compute/notebook
    use: [bucket]
    settings:
      name_prefix: notebook
      machine_type: n1-standard-4
      network_interfaces:
        - network: "projects/HOST_PROJECT_ID/global/networks/SHARED_VPC_NAME"
          subnet: "projects/HOST_PROJECT_ID/regions/REGION/subnetworks/SUBNET_NAME"
          nic_type: "VIRTIO_NET"
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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 5.34 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 5.34 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.mount_script](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_workbench_instance.instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/workbench_instance) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment; used as part of name of the notebook. | `string` | n/a | yes |
| <a name="input_gcs_bucket_path"></a> [gcs\_bucket\_path](#input\_gcs\_bucket\_path) | Bucket name, can be provided from the google-cloud-storage module | `string` | `null` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Instance Image | `map(string)` | <pre>{<br/>  "family": "tf-latest-cpu",<br/>  "name": null,<br/>  "project": "deeplearning-platform-release"<br/>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the resource Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | The machine type to employ | `string` | n/a | yes |
| <a name="input_mount_runner"></a> [mount\_runner](#input\_mount\_runner) | mount content from the google-cloud-storage module | `map(string)` | n/a | yes |
| <a name="input_network_interfaces"></a> [network\_interfaces](#input\_network\_interfaces) | A list of network interfaces for the VM instance. Each network interface is represented by an object with the following fields:<br/><br/>- network: (Optional) The name of the Virtual Private Cloud (VPC) network that this VM instance is connected to.<br/><br/>- subnet: (Optional) The name of the subnetwork within the specified VPC that this VM instance is connected to.<br/><br/>- nic\_type: (Optional) The type of vNIC to be used on this interface. Possible values are: `VIRTIO_NET`, `GVNIC`.<br/><br/>- access\_configs: (Optional) An array of access configurations for this network interface. The access\_config object contains:<br/>  * external\_ip: (Required) An external IP address associated with this instance. Specify an unused static external IP address available to the project or leave this field undefined to use an IP from a shared ephemeral IP address pool. If you specify a static external IP address, it must live in the same region as the zone of the instance. | <pre>list(object({<br/>    network  = optional(string)<br/>    subnet   = optional(string)<br/>    nic_type = optional(string)<br/>    access_configs = optional(list(object({<br/>      external_ip = optional(string)<br/>    })))<br/>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which the notebook will be created. | `string` | n/a | yes |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | If defined, the instance will use the service account specified instead of the Default Compute Engine Service Account | `string` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The zone to deploy to | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
