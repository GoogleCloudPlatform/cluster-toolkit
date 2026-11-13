# Instructions Module

This module enables blueprint authors to provide user-facing instructions and
next steps upon successful deployment of a blueprint. It supports dynamic
templating, optional decorative banners, local file generation (e.g.
`NEXT_STEPS.txt`), and integration with standard `instructions` outputs.

## Motivation

Blueprints frequently produce dynamic resources (such as Cloud Storage bucket
paths, cluster endpoints, or generated submission scripts) that users need in
order to execute their workloads. In some cases, blueprint authors may need to
provide additional details and instructions to the user. This module allows
blueprint authors to:

1. Render instructions dynamically from a template file (`templatefile()`) or raw string.
2. Present instructions natively at the end of `gcluster deploy` via the standard `instructions` output.
3. Optionally persist instructions to a local file in the deployment directory for ongoing reference.
4. Optionally print directly to the terminal console during `terraform apply`.

## Usage Examples

### Using a Template File with Blueprint Variables

```yaml
- group: batch
  modules:
  - id: next-steps
    source: community/modules/general/instructions
    settings:
      banner_title: "Next Steps"
      template_path: $(ghpc_stage("./instructions.txt.tftpl"))
      template_vars:
        bucket_path: $(my-bucket.gcs_bucket_path)
        deployment_name: $(vars.deployment_name)
      write_to_file: NEXT_STEPS.txt
    outputs:
      - instructions
```

### Using Inline String Content

```yaml
- group: instructions
  modules:
  - id: show-instructions
    source: community/modules/general/instructions
    settings:
      banner_title: "How to Access Your Cluster"
      content: |
        Connect to your login node with:
          gcloud compute ssh my-login-node --zone=us-central1-a
      write_to_file: README.txt
    outputs:
      - instructions
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_local"></a> [local](#requirement\_local) | >= 2.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_local"></a> [local](#provider\_local) | >= 2.0.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [local_file.instructions_file](https://registry.terraform.io/providers/hashicorp/local/latest/docs/resources/file) | resource |
| [terraform_data.print_instructions](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_banner_title"></a> [banner\_title](#input\_banner\_title) | Optional title to display in a decorative banner box around the instructions. | `string` | `null` | no |
| <a name="input_content"></a> [content](#input\_content) | Raw string content of instructions to display. If template\_path is also provided, template\_path takes precedence. | `string` | `null` | no |
| <a name="input_print_to_terminal"></a> [print\_to\_terminal](#input\_print\_to\_terminal) | Whether to print the formatted instructions directly to terminal during terraform apply via local-exec. | `bool` | `false` | no |
| <a name="input_template_path"></a> [template\_path](#input\_template\_path) | Path to a template file (.tftpl) to render using template\_vars. | `string` | `null` | no |
| <a name="input_template_vars"></a> [template\_vars](#input\_template\_vars) | Variables to pass to templatefile when template\_path is provided. | `any` | `{}` | no |
| <a name="input_write_to_file"></a> [write\_to\_file](#input\_write\_to\_file) | Optional destination file path on the local filesystem to write the formatted instructions. Relative paths resolve relative to the deployment root directory (${path.root}). | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_file_path"></a> [file\_path](#output\_file\_path) | Path of the local instructions file, if write\_to\_file was configured. |
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Formatted user instructions. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
