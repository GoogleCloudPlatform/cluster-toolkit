## Description
The tag module creates a tag key and the associated tag values.

If the key already exists, then the tag values passed are associated with the existing tag key.

The module creates of two resources; [google_tags_tag_key](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/tags_tag_key) and [google_tags_tag_value](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/tags_tag_value).

### Example
The following example creates a TagKey, TagValue, and LocationTagBinding resource.

```yaml
  - id: gke-h4d-fw-tags
    source: modules/management/tags
    settings:
      tag_key_parent: "projects/my-gcp-project"
      tag_key_short_name: "fw-falcon-tagkey"
      tag_key_description: "tagkey for firewall falcon VPC"
      tag_key_purpose: "GCE_FIREWALL"
      tag_key_purpose_data: "network=PROJECT_ID/NETWORK"
      tag_value:
        - short_name: "fw-falcon-tagvalue-1"
          description: "fw-falcon-tagvalue-1 is for purpose-1"
        - short_name: "fw-falcon-tagvalue-2"
          description: "fw-falcon-tagvalue-2 is for purpose-2"
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5 |
| <a name="requirement_external"></a> [external](#requirement\_external) | >= 2.3.5 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.2 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_external"></a> [external](#provider\_external) | >= 2.3.5 |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.2 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_tags_tag_key.key](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/tags_tag_key) | resource |
| [google_tags_tag_value.values](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/tags_tag_value) | resource |
| [external_external.check_tag_key](https://registry.terraform.io/providers/hashicorp/external/latest/docs/data-sources/external) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_tag_key_description"></a> [tag\_key\_description](#input\_tag\_key\_description) | User-assigned description of the TagKey. Must not exceed 256 characters. | `string` | `""` | no |
| <a name="input_tag_key_parent"></a> [tag\_key\_parent](#input\_tag\_key\_parent) | The resource name of the new TagKey's parent. Must be of the form organizations/{org\_id} or projects/{project\_id\_or\_number}. | `string` | n/a | yes |
| <a name="input_tag_key_purpose"></a> [tag\_key\_purpose](#input\_tag\_key\_purpose) | A purpose cannot be changed once set. A purpose denotes that this Tag is intended for use in policies of a specific policy engine, and will involve that policy engine in management operations involving this Tag. Possible values are: GCE\_FIREWALL, DATA\_GOVERNANCE. | `string` | `null` | no |
| <a name="input_tag_key_purpose_data"></a> [tag\_key\_purpose\_data](#input\_tag\_key\_purpose\_data) | Purpose data cannot be changed once set. Purpose data corresponds to the policy system that the tag is intended for. For example, the GCE\_FIREWALL purpose expects data in the following format: network = "<project-name>/<vpc-name>". | `map(string)` | `null` | no |
| <a name="input_tag_key_short_name"></a> [tag\_key\_short\_name](#input\_tag\_key\_short\_name) | The user friendly name for a TagKey. The short name should be unique for TagKeys within the same tag namespace. The short name can have a maximum length of 256 characters. The permitted character set for the shortName includes all UTF-8 encoded Unicode characters except single quotes ('), double quotes ("), backslashes (\), and forward slashes (/). | `string` | n/a | yes |
| <a name="input_tag_value"></a> [tag\_value](#input\_tag\_value) | A TagValue is a child of a particular TagKey. TagValues are used to group cloud resources for the purpose of controlling them using policies.<br/>short\_name:User-assigned short name for TagValue. The short name should be unique for TagValues within the same parent TagKey. The short name can have a maximum length of 256 characters. The permitted character set for the shortName includes all UTF-8 encoded Unicode characters except single quotes ('), double quotes (\"), backslashes (\\), and forward slashes (/).<br/>description: User-assigned description of the TagValue. Must not exceed 256 characters. | <pre>list(object({<br/>    short_name  = string<br/>    description = string<br/>  }))</pre> | `[]` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
