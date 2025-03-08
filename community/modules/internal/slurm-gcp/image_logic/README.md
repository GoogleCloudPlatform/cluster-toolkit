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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.4 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Defines the image that will be used in the Slurm VM instances.<br/><br/>Expected Fields:<br/>name: The name of the image. Mutually exclusive with family.<br/>family: The image family to use. Mutually exclusive with name.<br/>project: The project where the image is hosted.<br/><br/>For more information on creating custom images that comply with Slurm on GCP<br/>see the "Slurm on GCP Custom Images" section in docs/vm-images.md. | `map(string)` | <pre>{<br/>  "family": "slurm-gcp-6-8-hpc-rocky-linux-8",<br/>  "project": "schedmd-slurm-public"<br/>}</pre> | no |
| <a name="input_instance_image_custom"></a> [instance\_image\_custom](#input\_instance\_image\_custom) | A flag that designates that the user is aware that they are requesting<br/>to use a custom and potentially incompatible image for this Slurm on<br/>GCP module.<br/><br/>If the field is set to false, only the compatible families and project<br/>names will be accepted.  The deployment will fail with any other image<br/>family or name.  If set to true, no checks will be done.<br/><br/>See: https://goo.gle/hpc-slurm-images | `bool` | `false` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_source_image"></a> [source\_image](#output\_source\_image) | Image name |
| <a name="output_source_image_family"></a> [source\_image\_family](#output\_source\_image\_family) | Image family |
| <a name="output_source_image_project_normalized"></a> [source\_image\_project\_normalized](#output\_source\_image\_project\_normalized) | Normalized project id |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
