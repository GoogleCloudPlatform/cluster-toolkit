## Description
This resource creates a new VPC network along with a [cloud NAT](https://github.com/terraform-google-modules/terraform-google-cloud-nat),
[Router](https://github.com/terraform-google-modules/terraform-google-cloud-router)
and common [firewall rules](https://github.com/terraform-google-modules/terraform-google-network/tree/master/modules/firewall-rules).
This resource is based on submodules defined by the [Cloud Foundation Toolkit](https://cloud.google.com/foundation-toolkit).

The created cloud NAT (Network Address Translation) allows virtual machines
without external IP addresses create outbound connections to the internet. For
more information see the [docs](https://cloud.google.com/nat/docs/overview).

The following firewall rules are created with the VPC network:
* Allow SSH access from the Cloud Console ("35.235.240.0/20").
* Allow traffic between nodes within the VPC

### Example
```
- source: ./resources/network/vpc
  kind: terraform
  id: network1
  settings:
  - deployment_name: $(vars.deployment_name)
```
This creates a new VPC network named based on the `deployment_name` variable
with `_net` appended. `network_name` can be set manually as well as part of the
settings.

Note that `deployment_name` does not need to be set explicitly here,
it would typically be inferred from the global variable of the same name. It was
included for clarity.

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

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_project_service.gcp_services](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_service) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_gcp_service_list"></a> [gcp\_service\_list](#input\_gcp\_service\_list) | list of APIs to be enabled for the project | `list(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of the project | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->