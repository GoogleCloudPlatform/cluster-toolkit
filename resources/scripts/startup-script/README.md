## Description
This resource creates startup scripts by chaining together a list of provided
shell scripts and ansible configs, or "runners". These startup scripts can be
provided to compute VMs in their resource Settings.

Runners will be uploaded to a [GCS bucket](https://cloud.google.com/storage/docs/creating-buckets).
VMs using the startup script created by this resource will pull the runners from
that bucket, and therefore must have access to GCS.

### Example
```
- source: ./resources/scripts/startup-script
  kind: terraform
  id: startup
  settings:
    runners:
      - type: shell
        file: "modules/startup-script/examples/install_ansible.sh"
      - type: shell
        file: "modules/filestore/scripts/install-nfs.sh"
      - type: ansible-local
        file: "modules/startup-script/examples/mount.yaml"

- source: ./resources/compute/simple-instance
  kind: terraform
  id: compute-cluster
  settings:
    network_storage:
    - $(homefs.network_storage)
    metadata:
      startup-script: $(startup.startup_script_content)
      startup-script-custom: $(startup.startup_script_custom_content)
```

## License
<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2021 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 3.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 3.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_startup_scripts"></a> [startup\_scripts](#module\_startup\_scripts) | github.com/terraform-google-modules/terraform-google-startup-scripts | v1.0.0 |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket.configs_bucket](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket) | resource |
| [google_storage_bucket_object.scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used to name GCS bucket for startup scripts. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region to deploy to | `string` | n/a | yes |
| <a name="input_runners"></a> [runners](#input\_runners) | List of runners to run on remote VM | <pre>list(object({<br>    type = string,<br>    file = string,<br>  }))</pre> | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_startup_script_content"></a> [startup\_script\_content](#output\_startup\_script\_content) | startup-script-stdlib.sh content as a string value. |
| <a name="output_startup_script_custom_content"></a> [startup\_script\_custom\_content](#output\_startup\_script\_custom\_content) | Custom startup script to load and run all runners. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
