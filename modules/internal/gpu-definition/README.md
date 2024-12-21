<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2024 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br/>    type  = string,<br/>    count = number<br/>  }))</pre> | `[]` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for the instance creation | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_guest_accelerator"></a> [guest\_accelerator](#output\_guest\_accelerator) | Sanitized list of the type and count of accelerator cards attached to the instance. |
| <a name="output_machine_type_guest_accelerator"></a> [machine\_type\_guest\_accelerator](#output\_machine\_type\_guest\_accelerator) | List of the type and count of accelerator cards attached to the specified machine type. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
