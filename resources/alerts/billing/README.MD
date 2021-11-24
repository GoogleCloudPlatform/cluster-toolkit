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

| Name                                                         | Type        |
| ------------------------------------------------------------ | ----------- |
| [google_billing_account](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/billing_account) | resource    |
| [google_project_service](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/google_project) | data source |
| [google_billing_budget.budget](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/billing_budget) | data source |

## Inputs

| Name | Description | Type | Default |
|------|-------------|------|---------|
| <a name="input_bill_account"></a> [bill\_account](#input\_bill\_account) | billing account for which alerts should be sent. | `string` | n/a |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project id for which APIs will be enabled | `string` | n/a     |
| <a name="input_budget_name"></a> [budget\_name](#input\_budget\_name) | Alias name for the budget | `string` | n/a |
| <a name="input_amount"></a> [amount](#input\_amount) | The amount to use as the budget | `string` | 1000 |
| <a name="input_currency"></a> [currency](#input\_currency) | The currency to use for the budget | `string` | USD |
| <a name="input_email"></a> [email](#input\_email) | The email for notifications | `string` | zi.wangs@hcl.com |
| <a name="input_percent1"></a> [percent1](#input\_percent1) | A list of percentages of the budget to alert on when threshold is exceeded | `number` | 0.5 |
| <a name="input_percent2"></a> [percent2](#input\_percent2) | A list of percentages of the budget to alert on when threshold is exceeded | `number` | 0.75 |
| <a name="input_percent3"></a> [percent3](#input\_percent3) | A list of percentages of the budget to alert on when threshold is exceeded | `number` | 0.8 |
| <a name="input_percent4"></a> [percent4](#input\_percent4) | A list of percentages of the budget to alert on when threshold is exceeded | `number` | 1 |

