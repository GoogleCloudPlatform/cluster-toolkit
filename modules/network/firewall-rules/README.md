## Description

This module facilitates the creation of custom firewall rules for existing
networks.

## Example usage

This module can be used by other Toolkit modules to create application-specific
firewall rules or in conjunction with the [pre-existing-vpc] module to enable
traffic in existing networks. The snippet below is drawn from the
[ml-slurm.yaml] example:

```yaml
- group: primary
  modules:
  - id: network
    source: modules/network/pre-existing-vpc

  # this example anticipates that the VPC default network has internal traffic
  # allowed and IAP tunneling for SSH connections
  - id: firewall_rule
    source: modules/network/firewall-rules
    use:
    - network
    settings:
      ingress_rules:
      - name: $(vars.deployment_name)-allow-internal-traffic
        description: Allow internal traffic
        destination_ranges:
        - $(network.subnetwork_address)
        source_ranges:
        - $(network.subnetwork_address)
        allow:
        - protocol: tcp
          ports:
          - 0-65535
        - protocol: udp
          ports:
          - 0-65535
        - protocol: icmp
      - name: $(vars.deployment_name)-allow-iap-ssh
        description: Allow IAP-tunneled SSH connections
        destination_ranges:
        - $(network.subnetwork_address)
        source_ranges:
        - 35.235.240.0/20
        allow:
        - protocol: tcp
          ports:
          - 22
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_firewall_rule"></a> [firewall\_rule](#module\_firewall\_rule) | terraform-google-modules/network/google//modules/firewall-rules | ~> 12.0 |

## Resources

| Name | Type |
|------|------|
| [terraform_data.pga_check](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [google_compute_subnetwork.subnetwork](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_egress_rules"></a> [egress\_rules](#input\_egress\_rules) | List of egress rules | <pre>list(object({<br/>    name                    = string<br/>    description             = optional(string, null)<br/>    disabled                = optional(bool, null)<br/>    priority                = optional(number, null)<br/>    destination_ranges      = optional(list(string), [])<br/>    source_ranges           = optional(list(string), [])<br/>    source_tags             = optional(list(string))<br/>    source_service_accounts = optional(list(string))<br/>    target_tags             = optional(list(string))<br/>    target_service_accounts = optional(list(string))<br/><br/>    allow = optional(list(object({<br/>      protocol = string<br/>      ports    = optional(list(string))<br/>    })), [])<br/>    deny = optional(list(object({<br/>      protocol = string<br/>      ports    = optional(list(string))<br/>    })), [])<br/>    log_config = optional(object({<br/>      metadata = string<br/>    }))<br/>  }))</pre> | `[]` | no |
| <a name="input_ingress_rules"></a> [ingress\_rules](#input\_ingress\_rules) | List of ingress rules | <pre>list(object({<br/>    name                    = string<br/>    description             = optional(string, null)<br/>    disabled                = optional(bool, null)<br/>    priority                = optional(number, null)<br/>    destination_ranges      = optional(list(string), [])<br/>    source_ranges           = optional(list(string), [])<br/>    source_tags             = optional(list(string))<br/>    source_service_accounts = optional(list(string))<br/>    target_tags             = optional(list(string))<br/>    target_service_accounts = optional(list(string))<br/><br/>    allow = optional(list(object({<br/>      protocol = string<br/>      ports    = optional(list(string))<br/>    })), [])<br/>    deny = optional(list(object({<br/>      protocol = string<br/>      ports    = optional(list(string))<br/>    })), [])<br/>    log_config = optional(object({<br/>      metadata = string<br/>    }))<br/>  }))</pre> | `[]` | no |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | The name of the network to create firewall rules in | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to host the network in | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork whose global network firewall rules will be modified. | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->

[pre-existing-vpc]: ../pre-existing-vpc/README.md
[ml-slurm.yaml]: ../../../examples/ml-slurm.yaml
