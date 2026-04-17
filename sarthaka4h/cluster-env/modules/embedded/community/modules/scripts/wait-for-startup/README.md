## Description

This module will insert a dependency on the completion of the startup script
for one or more specified compute VMs and report back if it fails. This can be useful when running
post-boot installation scripts that require the startup script to finish setting up a node.

> **_WARNING:_**: this module is experimental and not fully supported.

### Additional Dependencies

* [**gcloud**](https://cloud.google.com/sdk/gcloud) must be present in the path
  of the machine where `terraform apply` is run.

### Example

```yaml
- id: workstation
  source: modules/compute/vm-instance
  use:
  - network1
  - my-startup-script
  settings:
    instance_count: 4

# Wait for all instances of the above VM to finish running startup scripts.
- id: wait
  source: community/modules/scripts/wait-for-startup
  settings:
    instance_names: $(workstation.name)
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [null_resource.validate_instance_names](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [null_resource.wait_for_startup](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_gcloud_path_override"></a> [gcloud\_path\_override](#input\_gcloud\_path\_override) | Directory of the gcloud executable to be used during cleanup | `string` | `""` | no |
| <a name="input_instance_name"></a> [instance\_name](#input\_instance\_name) | Name of the instance we are waiting for (can be null if 'instance\_names' is not empty) | `string` | `null` | no |
| <a name="input_instance_names"></a> [instance\_names](#input\_instance\_names) | A list of instance names we are waiting for, in addition to the one mentioned in 'instance\_name' (if any) | `list(string)` | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_timeout"></a> [timeout](#input\_timeout) | Timeout in seconds | `number` | `1200` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The GCP zone where the instance is running | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
