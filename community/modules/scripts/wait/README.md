## Description

This module allows setting a dependency and sleep functionality for other resources. It is a thin wrapper around
[time_sleep Terraform resource](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/sleep)
It may be used  to delay creation of resources after some time of other resources completing. This is useful, if
configuration of resources is done outside the Terraform.

`create_duration` provides wait functionality of wait during the resource creation, `destroy_duration` provides wait
functionality for resource removal. `triggers` is a map, that allows specifying dependencies of the resource, that is
only once those resources are known wait is triggered. Any change to this map will re-trigger wait.

Module provides `empty` output, that is always empty string, that can be used to introduce wait before provisioning
of the other resource

### Example

```yaml
  - id: wait
    source: community/modules/scripts/wait
    settings:
      create_duration: "10m"
      triggers:
        dependency: $(other_module.attribute)

  - id: startup
    source: modules/scripts/startup-script
    settings:
      runners:
        # Some modules such as filestore have runners as outputs for convenience:
        - $(homefs.install_nfs_client_runner)
        - type: shell
          destination: "run.sh$(wait.empty)"
          content: |
            #!/bin/sh
            echo Will wait 10 minutes, before creating this startup script
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2025 Google LLC

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
| <a name="requirement_time"></a> [time](#requirement\_time) | >= 0.9.1 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_time"></a> [time](#provider\_time) | >= 0.9.1 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [time_sleep.wait](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/sleep) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_create_duration"></a> [create\_duration](#input\_create\_duration) | Time duration to delay resource creation. For example, 30s for 30 seconds or 5m for 5 minutes. Updating this value by itself will not trigger a delay. | `string` | `null` | no |
| <a name="input_destroy_duration"></a> [destroy\_duration](#input\_destroy\_duration) | Time duration to delay resource destroy. For example, 30s for 30 seconds or 5m for 5 minutes. Updating this value by itself will not trigger a delay. This value or any updates to it must be successfully applied into the Terraform state before destroying this resource to take effect. | `string` | `null` | no |
| <a name="input_triggers"></a> [triggers](#input\_triggers) | (Optional) Arbitrary map of values that, when changed, will run any creation or destroy delays again. | `map(string)` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_empty"></a> [empty](#output\_empty) | Empty string that depends on the wait. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
