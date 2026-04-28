## Description

This module creates a Google Cloud DNS Managed Zone.

## Example usage

```yaml
- id: dns_managed_zone
  source: modules/network/dns-managed-zone
  settings:
    project_id: $(vars.project_id)
    zone_name: my-zone
    dns_name: example.com.
    description: "My DNS Zone"
    labels:
      env: "dev"
    recordsets:
    - name: "www"
      type: "A"
      ttl: 300
      rrdatas:
      - "1.2.3.4"
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.73.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.73.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_dns_managed_zone.zone](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/dns_managed_zone) | resource |
| [google_dns_record_set.record](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/dns_record_set) | resource |
| [google_project_service.dns_api](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_service) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_description"></a> [description](#input\_description) | A textual description of this managed zone | `string` | `"Managed by Cluster Toolkit"` | no |
| <a name="input_dns_name"></a> [dns\_name](#input\_dns\_name) | The DNS name of this managed zone, e.g. 'example.com.' | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | A set of key/value label pairs to assign to this ManagedZone | `map(string)` | `{}` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID | `string` | n/a | yes |
| <a name="input_recordsets"></a> [recordsets](#input\_recordsets) | List of DNS record sets to create in the zone | <pre>list(object({<br/>    name    = string<br/>    type    = string<br/>    ttl     = number<br/>    rrdatas = list(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_zone_name"></a> [zone\_name](#input\_zone\_name) | The name of the DNS zone | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_managed_zone_id"></a> [managed\_zone\_id](#output\_managed\_zone\_id) | The fully qualified ID of the DNS Managed Zone. |
| <a name="output_name_servers"></a> [name\_servers](#output\_name\_servers) | The delegated name servers for the zone. |
| <a name="output_zone_name"></a> [zone\_name](#output\_zone\_name) | The name of the managed DNS zone. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
