[<img src="https://raw.githubusercontent.com/mineiros-io/brand/3bffd30e8bdbbde32c143e2650b2faa55f1df3ea/mineiros-primary-logo.svg" width="400"/>](https://mineiros.io/?ref=terraform-google-billing-budget)

[![Build Status](https://github.com/mineiros-io/terraform-google-billing-budget/workflows/Tests/badge.svg)](https://github.com/mineiros-io/terraform-google-billing-budget/actions)
[![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/mineiros-io/terraform-google-billing-budget.svg?label=latest&sort=semver)](https://github.com/mineiros-io/terraform-google-billing-budget/releases)
[![Terraform Version](https://img.shields.io/badge/Terraform-1.x-623CE4.svg?logo=terraform)](https://github.com/hashicorp/terraform/releases)
[![Google Provider Version](https://img.shields.io/badge/google-4-1A73E8.svg?logo=terraform)](https://github.com/terraform-providers/terraform-provider-google/releases)
[![Join Slack](https://img.shields.io/badge/slack-@mineiros--community-f32752.svg?logo=slack)](https://mineiros.io/slack)

# terraform-google-billing-budget

A [Terraform] module to manage [Google Billing Budgets](https://cloud.google.com/billing/docs/how-to/budgets) on [Google Cloud Platform (GCP)](https://cloud.google.com).

A budget enables you to track your actual Google Cloud spend against your
planned spend. After you've set a budget amount, you set budget alert
threshold rules that are used to trigger notifications such as email, slack and Pub/Sub.

**_This module supports Terraform version 1
and is compatible with the Terraform Google Cloud Provider version 4._**

This module is part of our Infrastructure as Code (IaC) framework
that enables our users and customers to easily deploy and manage reusable,
secure, and production-grade cloud infrastructure.


- [Module Features](#module-features)
- [Getting Started](#getting-started)
- [Module Argument Reference](#module-argument-reference)
  - [Top-level Arguments](#top-level-arguments)
    - [Main Resource Configuration](#main-resource-configuration)
    - [Module Configuration](#module-configuration)
- [Module Attributes Reference](#module-attributes-reference)
- [External Documentation](#external-documentation)
  - [GCP Billing Budgets Documentation](#gcp-billing-budgets-documentation)
  - [Terraform GCP Provider Documentation](#terraform-gcp-provider-documentation)
- [Module Versioning](#module-versioning)
  - [Backwards compatibility in `0.0.z` and `0.y.z` version](#backwards-compatibility-in-00z-and-0yz-version)
- [About Mineiros](#about-mineiros)
- [Reporting Issues](#reporting-issues)
- [Contributing](#contributing)
- [Makefile Targets](#makefile-targets)
- [License](#license)

## Module Features

This module implements the following Terraform resources

- `google_billing_budget`

## Getting Started

Most common usage of the module:

```hcl
module "terraform-google-billing-budget" {
  source  = "mineiros-io/billing-budget/google"
  version = "0.0.3"

  display_name    = "example-alert"
  billing_account = "xxxxxxxx-xxxx-xxxxxxx"
  amount          = 1000
  currency_code   = "EUR"
  treshold_rules = [
    {
      threshold_percent = 1.0
    },
    {
      threshold_percent = 1.0
      spend_basis       = "FORECASTED_SPEND"
    }
  ]
}
```

## Module Argument Reference

See [variables.tf] and [examples/] for details and use-cases.

### Top-level Arguments

#### Main Resource Configuration

- [**`billing_account`**](#var-billing_account): *(**Required** `string`)*<a name="var-billing_account"></a>

  ID of the billing account to set a budget on.

- [**`threshold_rules`**](#var-threshold_rules): *(Optional `list(threshold_rules)`)*<a name="var-threshold_rules"></a>

  Example:

  ```hcl
  treshold_rules = [
    {
      threshold_percent = 1.0
    },
    {
      threshold_percent = 1.0
      spend_basis       = "FORECASTED_SPEND"
    }
  ]
  ```

  Each `threshold_rules` object in the list accepts the following attributes:

  - [**`threshold_percent`**](#attr-threshold_rules-threshold_percent): *(**Required** `number`)*<a name="attr-threshold_rules-threshold_percent"></a>

    Send an alert when this threshold is exceeded. This is a 1.0-based percentage, so 0.5 = 50%. Must be >= 0.

  - [**`spend_basis`**](#attr-threshold_rules-spend_basis): *(Optional `string`)*<a name="attr-threshold_rules-spend_basis"></a>

    The type of basis used to determine if spend has passed the threshold. Default value is `CURRENT_SPEND`. Possible values are `CURRENT_SPEND` and `FORECASTED_SPEND`.

- [**`amount`**](#var-amount): *(Optional `number`)*<a name="var-amount"></a>

  A specified amount to use as the budget.

  Default is `null`.

- [**`currency_code`**](#var-currency_code): *(Optional `string`)*<a name="var-currency_code"></a>

  The 3-letter currency code defined in ISO 4217. If specified, it must match the currency of the billing account. For a list of currency codes, please see https://en.wikipedia.org/wiki/ISO_4217

  Default is `null`.

- [**`use_last_period_amount`**](#var-use_last_period_amount): *(Optional `bool`)*<a name="var-use_last_period_amount"></a>

  If set to `true`, the amount of the budget will be dynamically set and updated based on the last calendar period's spend.

  Default is `false`.

- [**`display_name`**](#var-display_name): *(Optional `string`)*<a name="var-display_name"></a>

  The name of the budget that will be displayed in the GCP console. Must be <= 60 chars.

  Default is `null`.

- [**`budget_filter`**](#var-budget_filter): *(Optional `object(budget_filter)`)*<a name="var-budget_filter"></a>

  Filters that define which resources are used to compute the actual spend against the budget.

  Default is `null`.

  Example:

  ```hcl
  budget_filter = {
    projects               = ["projects/xxx"]
    credit_types_treatment = "INCLUDE_SPECIFIED_CREDITS"
    credit_types           = "COMMITTED_USAGE_DISCOUNT"
    services               = ["services/example-service"]
    subaccounts            = ["billingAccounts/xxx"]
    labels                 = {
      Environment = "Dev"
    }
  }
  ```

  The `budget_filter` object accepts the following attributes:

  - [**`projects`**](#attr-budget_filter-projects): *(Optional `set(string)`)*<a name="attr-budget_filter-projects"></a>

    A set of projects of the form `projects/{project_number}`, specifying that usage from only this set of projects should be included in the budget. If omitted, the report will include all usage for the billing account, regardless of which project the usage occurred on.

    Default is `null`.

  - [**`credit_types_treatment`**](#attr-budget_filter-credit_types_treatment): *(Optional `string`)*<a name="attr-budget_filter-credit_types_treatment"></a>

    Specifies how credits should be treated when determining spend for threshold calculations. Possible values are `INCLUDE_ALL_CREDITS`, `EXCLUDE_ALL_CREDITS`, and `INCLUDE_SPECIFIED_CREDITS`.

    Default is `"INCLUDE_ALL_CREDITS"`.

  - [**`credit_types`**](#attr-budget_filter-credit_types): *(Optional `string`)*<a name="attr-budget_filter-credit_types"></a>

    If `credit_types_treatment` is set to `INCLUDE_SPECIFIED_CREDITS`, this is a list of credit types to be subtracted from gross cost to determine the spend for threshold calculations. See [a list of acceptable credit type values](https://cloud.google.com/billing/docs/how-to/export-data-bigquery-tables#credits-type)

    Default is `null`.

  - [**`services`**](#attr-budget_filter-services): *(Optional `set(string)`)*<a name="attr-budget_filter-services"></a>

    A set of services of the form `services/{service_id}`, specifying that usage from only this set of services should be included in the budget. If omitted, the report will include usage for all the services. For a list of available services please see: https://cloud.google.com/billing/v1/how-tos/catalog-api.

    Default is `null`.

  - [**`subaccounts`**](#attr-budget_filter-subaccounts): *(Optional `set(string)`)*<a name="attr-budget_filter-subaccounts"></a>

    A set of subaccounts of the form `billingAccounts/{account_id}`, specifying that usage from only this set of subaccounts should be included in the budget. If a subaccount is set to the name of the parent account, usage from the parent account will be included. If the field is omitted, the report will include usage from the parent account and all subaccounts, if they exist.

    Default is `null`.

  - [**`labels`**](#attr-budget_filter-labels): *(Optional `map(string)`)*<a name="attr-budget_filter-labels"></a>

    A single label and value pair specifying that usage from only this set of labeled resources should be included in the budget.

    Default is `null`.

- [**`notifications`**](#var-notifications): *(Optional `object(notifications)`)*<a name="var-notifications"></a>

  Defines notifications that are sent on every update to the billing account's spend, regardless of the thresholds defined using threshold rules.

  Default is `null`.

  Example:

  ```hcl
  notifications = {
    pubsub_topic                     = "alert-notification-topic"
    monitoring_notification_channels = [
      "projects/sample-project/example-alert-notification",
    ]
    disable_default_iam_recipients   = true
  }
  ```

  The `notifications` object accepts the following attributes:

  - [**`pubsub_topic`**](#attr-notifications-pubsub_topic): *(Optional `string`)*<a name="attr-notifications-pubsub_topic"></a>

    The name of the Cloud Pub/Sub topic where budget related messages will be published, in the form `projects/{project_id}/topics/{topic_id}`. Updates are sent at regular intervals to the topic.

    Default is `null`.

  - [**`schema_version`**](#attr-notifications-schema_version): *(Optional `string`)*<a name="attr-notifications-schema_version"></a>

    The schema version of the notification. It represents the JSON schema as defined in https://cloud.google.com/billing/docs/how-to/budgets#notification_format.

    Default is `"1.0"`.

  - [**`monitoring_notification_channels`**](#attr-notifications-monitoring_notification_channels): *(Optional `set(string)`)*<a name="attr-notifications-monitoring_notification_channels"></a>

    The full resource name of a monitoring notification channel in the form `projects/{project_id}/notificationChannels/{channel_id}`. A maximum of 5 channels are allowed.

    Default is `null`.

  - [**`disable_default_iam_recipients`**](#attr-notifications-disable_default_iam_recipients): *(Optional `bool`)*<a name="attr-notifications-disable_default_iam_recipients"></a>

    When set to true, disables default notifications sent when a threshold is exceeded. Default recipients are those with Billing Account Administrators and Billing Account Users IAM roles for the target account.

    Default is `null`.

#### Module Configuration

- [**`module_enabled`**](#var-module_enabled): *(Optional `bool`)*<a name="var-module_enabled"></a>

  Specifies whether resources in the module will be created.

  Default is `true`.

- [**`module_timeouts`**](#var-module_timeouts): *(Optional `object(google_billing_budget)`)*<a name="var-module_timeouts"></a>

  How long certain operations (per resource type) are allowed to take before being considered to have failed.

  Default is `{}`.

  Example:

  ```hcl
  module_timeouts = {
    google_billing_budget = {
      create = "4m"
      update = "4m"
      delete = "4m"
    }
  }
  ```

  The `google_billing_budget` object accepts the following attributes:

  - [**`google_billing_budget`**](#attr-module_timeouts-google_billing_budget): *(Optional `object(timeouts)`)*<a name="attr-module_timeouts-google_billing_budget"></a>

    Timeout for the `google_billing_budget` resource.

    Default is `null`.

    The `timeouts` object accepts the following attributes:

    - [**`create`**](#attr-module_timeouts-google_billing_budget-create): *(Optional `string`)*<a name="attr-module_timeouts-google_billing_budget-create"></a>

      Timeout for `create` operations.

      Default is `null`.

    - [**`update`**](#attr-module_timeouts-google_billing_budget-update): *(Optional `string`)*<a name="attr-module_timeouts-google_billing_budget-update"></a>

      Timeout for `update` operations.

      Default is `null`.

    - [**`delete`**](#attr-module_timeouts-google_billing_budget-delete): *(Optional `string`)*<a name="attr-module_timeouts-google_billing_budget-delete"></a>

      Timeout for `delete` operations.

      Default is `null`.

- [**`module_depends_on`**](#var-module_depends_on): *(Optional `list(dependencies)`)*<a name="var-module_depends_on"></a>

  A list of dependencies. Any object can be _assigned_ to this list to define a hidden external dependency.

  Example:

  ```hcl
  module_depends_on = [
    google_monitoring_notification_channel.notification_channel 
  ]
  ```

## Module Attributes Reference

The following attributes are exported in the outputs of the module:

- **`module_enabled`**

  Whether this module is enabled.

## External Documentation

### GCP Billing Budgets Documentation

- https://cloud.google.com/billing/docs/how-to/budgets

### Terraform GCP Provider Documentation

- https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/billing_budget

## Module Versioning

This Module follows the principles of [Semantic Versioning (SemVer)].

Given a version number `MAJOR.MINOR.PATCH`, we increment the:

1. `MAJOR` version when we make incompatible changes,
2. `MINOR` version when we add functionality in a backwards compatible manner, and
3. `PATCH` version when we make backwards compatible bug fixes.

### Backwards compatibility in `0.0.z` and `0.y.z` version

- Backwards compatibility in versions `0.0.z` is **not guaranteed** when `z` is increased. (Initial development)
- Backwards compatibility in versions `0.y.z` is **not guaranteed** when `y` is increased. (Pre-release)

## About Mineiros

[Mineiros][homepage] is a remote-first company headquartered in Berlin, Germany
that solves development, automation and security challenges in cloud infrastructure.

Our vision is to massively reduce time and overhead for teams to manage and
deploy production-grade and secure cloud infrastructure.

We offer commercial support for all of our modules and encourage you to reach out
if you have any questions or need help. Feel free to email us at [hello@mineiros.io] or join our
[Community Slack channel][slack].

## Reporting Issues

We use GitHub [Issues] to track community reported issues and missing features.

## Contributing

Contributions are always encouraged and welcome! For the process of accepting changes, we use
[Pull Requests]. If you'd like more information, please see our [Contribution Guidelines].

## Makefile Targets

This repository comes with a handy [Makefile].
Run `make help` to see details on each available target.

## License

[![license][badge-license]][apache20]

This module is licensed under the Apache License Version 2.0, January 2004.
Please see [LICENSE] for full details.

Copyright &copy; 2020-2022 [Mineiros GmbH][homepage]


<!-- References -->

[homepage]: https://mineiros.io/?ref=terraform-google-billing-budget
[hello@mineiros.io]: mailto:hello@mineiros.io
[badge-license]: https://img.shields.io/badge/license-Apache%202.0-brightgreen.svg
[releases-terraform]: https://github.com/hashicorp/terraform/releases
[apache20]: https://opensource.org/licenses/Apache-2.0
[slack]: https://mineiros.io/slack
[terraform]: https://www.terraform.io
[gcp]: https://cloud.google.com
[semantic versioning (semver)]: https://semver.org/
[variables.tf]: https://github.com/mineiros-io/terraform-google-billing-budget/blob/main/variables.tf
[examples/]: https://github.com/mineiros-io/terraform-google-billing-budget/blob/main/examples
[issues]: https://github.com/mineiros-io/terraform-google-billing-budget/issues
[license]: https://github.com/mineiros-io/terraform-google-billing-budget/blob/main/LICENSE
[makefile]: https://github.com/mineiros-io/terraform-google-billing-budget/blob/main/Makefile
[pull requests]: https://github.com/mineiros-io/terraform-google-billing-budget/pulls
[contribution guidelines]: https://github.com/mineiros-io/terraform-google-billing-budget/blob/main/CONTRIBUTING.md
