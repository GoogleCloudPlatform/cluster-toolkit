
## Description

This module creates a [Billing Budget](https://cloud.google.com/billing/docs/how-to/budgets).

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | ~> 1.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 4.6 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 4.6 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_billing_budget.budget](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/billing_budget) | resource |
| [google_monitoring_notification_channel.manager_notification_channel](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/monitoring_notification_channel) | resource |
| [google_monitoring_notification_channel.scientist_notification_channel](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/monitoring_notification_channel) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_amount"></a> [amount](#input\_amount) | A specified amount to use as the budget. | `number` | n/a | yes |
| <a name="input_billing_account"></a> [billing\_account](#input\_billing\_account) | (Required) ID of the billing account to set a budget on. | `string` | n/a | yes |
| <a name="input_budget_end_date_day"></a> [budget\_end\_date\_day](#input\_budget\_end\_date\_day) | Day of a month to end budget with. Must be from 1 to 31 and valid for the year and month | `number` | n/a | yes |
| <a name="input_budget_end_date_month"></a> [budget\_end\_date\_month](#input\_budget\_end\_date\_month) | Month of a year to end budget with. Must be from 1 to 12 | `number` | n/a | yes |
| <a name="input_budget_end_date_year"></a> [budget\_end\_date\_year](#input\_budget\_end\_date\_year) | Year of the date to end budget with. Must be from 1 to 9999 | `number` | n/a | yes |
| <a name="input_budget_start_date_day"></a> [budget\_start\_date\_day](#input\_budget\_start\_date\_day) | Day of a month to start budget with. Must be from 1 to 31 and valid for the year and month | `number` | n/a | yes |
| <a name="input_budget_start_date_month"></a> [budget\_start\_date\_month](#input\_budget\_start\_date\_month) | Month of a year to start budget with. Must be from 1 to 12 | `number` | n/a | yes |
| <a name="input_budget_start_date_year"></a> [budget\_start\_date\_year](#input\_budget\_start\_date\_year) | Year of the date to start budget with. Must be from 1 to 9999 | `number` | n/a | yes |
| <a name="input_currency_code"></a> [currency\_code](#input\_currency\_code) | The 3-letter currency code defined in ISO 4217. If specified, it must match the currency of the billing account. For a list of currency codes, please see https://en.wikipedia.org/wiki/ISO_4217 | `string` | n/a | yes |
| <a name="input_manager"></a> [manager](#input\_manager) | Manager who approved the project | `string` | n/a | yes |
| <a name="input_module_depends_on"></a> [module\_depends\_on](#input\_module\_depends\_on) | (Optional) A list of external resources the module depends on. | `any` | `[]` | no |
| <a name="input_module_enabled"></a> [module\_enabled](#input\_module\_enabled) | (Optional) Whether to create resources within the module or not. | `bool` | `true` | no |
| <a name="input_module_timeouts"></a> [module\_timeouts](#input\_module\_timeouts) | (Optional) How long certain operations (per resource type) are allowed to take before being considered to have failed. | `any` | `{}` | no |
| <a name="input_owner"></a> [owner](#input\_owner) | Owner of the project | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | project ID | `string` | n/a | yes |
| <a name="input_threshold_rules"></a> [threshold\_rules](#input\_threshold\_rules) | (Required) Rules that trigger alerts (notifications of thresholds being crossed) when spend exceeds the specified percentages of the budget. | `list(number)` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_budget"></a> [budget](#output\_budget) | All attributes of the created `google_billing_budget` resource. |
| <a name="output_module_enabled"></a> [module\_enabled](#output\_module\_enabled) | Whether the module is enabled. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
