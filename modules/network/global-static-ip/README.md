## Description

This module creates Google Cloud Global Static IP addresses.

## Example usage

```yaml
- id: global_static_ip
  source: modules/network/global-static-ip
  settings:
    ip_names:
    - $(vars.deployment_name)-ip-1
    - $(vars.deployment_name)-ip-2
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_global_address.ips](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_global_address) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_ip_names"></a> [ip\_names](#input\_ip\_names) | List of global static IP names to create | `list(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP Project ID | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_global_ips"></a> [global\_ips](#output\_global\_ips) | A map of IP names to allocated global static IP addresses. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
