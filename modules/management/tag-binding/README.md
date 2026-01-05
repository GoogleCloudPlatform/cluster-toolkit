## Description

A LocationTagBinding represents a connection between a TagValue and a non-global target such as a Cloud Run Service or Compute Instance. Once a LocationTagBinding is created, the TagValue is applied to all the descendants of the cloud resource.

The module creates multiple tag-bindings from a list of parent resources, tag_values, and locations.

### Example
The following example creates a LocationTagBinding between a TagValue and a Compute Instance.

```yaml
  - id: gke-h4d-fw-tag-binding
    source: modules/management/tag-binding
    settings:
      tag_binding:
        - parent: "//compute.googleapis.com/projects/${google_project.project.number}/zones/us-central1-a/instances/<instance-id>" # The full resource name of the resource the TagValue is bound to. E.g. //cloudresourcemanager.googleapis.com/projects/123
          tag_value: tagValues/456 # The TagValue of the TagBinding. Must be of the form tagValues/456.
          location: "us-central1-a" # Location of the target resource.
        - parent: "//container.googleapis.com/projects/PROJECT_NUMBER/locations/LOCATION/clusters/CLUSTER_NAME/nodePools/NODE_POOL_NAME"
          tag_value: tagValues/456 
          location: "us-central1-a"
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.2 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.2 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_tags_location_tag_binding.binding](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/tags_location_tag_binding) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_tag_binding"></a> [tag\_binding](#input\_tag\_binding) | A list of LocationTagBindings to create. A LocationTagBinding represents a connection between a TagValue and a non-global target such as a Cloud Run Service or Compute Instance. Once a LocationTagBinding is created, the TagValue is applied to all the descendants of the cloud resource.<br/>Each object in the list should have the following attributes:<br/>- `parent`: The full resource name of the resource the TagValue is bound to. E.g. //cloudresourcemanager.googleapis.com/projects/123.<br/>- `tag_value`: The TagValue of the TagBinding. Must be of the form tagValues/456.<br/>- `location`: Location of the target resource. | <pre>list(object({<br/>    parent    = string<br/>    tag_value = string<br/>    location  = string<br/>  }))</pre> | `[]` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
