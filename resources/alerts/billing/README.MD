BILLING:

This source provides alerts on mail at chosen thresholds in terms of forecasted and actual spends on the project for example it can trigger an email alert when budget is exhausted to the levels like 25%, 50% and 75%.


Arguments Required:

amount -  The budgeted amount for each usage period.

display_name - User data for display name in UI. Must be <= 60 chars.

billing_account -  ID of the billing account to set a budget on.

specified_amount - A specified amount to use as the budget. currencyCode is optional. If specified, it must match the currency of the billing account. The currencyCode is provided on output.

currency_code - The 3-letter currency code defined in ISO 4217.

units - The whole units of the amount. For example if currencyCode is "USD", then 1 unit is one US dollar.

threshold_rules -  Rules that trigger alerts (notifications of thresholds being crossed) when spend exceeds the specified percentages of the budget.

threshold_rules block supports:

threshold_percent - Send an alert when this threshold is exceeded. This is a 1.0-based percentage, so 0.5 = 50%. Must be >= 0.

spend_basis - The type of basis used to determine if spend has passed the threshold. Default value is CURRENT_SPEND. Possible values are CURRENT_SPEND and FORECASTED_SPEND.


budget_filter -  Filters that define which resources are used to compute the actual spend against the budget. Structure is documented below.

The budget_filter block supports:

projects -  A set of projects of the form projects/{project_number}, specifying that usage from only this set of projects should be included in the budget. If omitted, the report will include all usage for the billing account, regardless of which project the usage occurred on.

credit_types_treatment - Specifies how credits should be treated when determining spend for threshold calculations. Default value is INCLUDE_ALL_CREDITS. Possible values are INCLUDE_ALL_CREDITS, EXCLUDE_ALL_CREDITS, and INCLUDE_SPECIFIED_CREDITS.

all_updates_rule - Defines notifications that are sent on every update to the billing account's spend, regardless of the thresholds defined using threshold rules.

monitoring_notification_channels - The full resource name of a monitoring notification channel in the form projects/{project_id}/notificationChannels/{channel_id}. A maximum of 5 channels are allowed.

disable_default_iam_recipients - Boolean. When set to true, disables default notifications sent when a threshold is exceeded. Default recipients are those with Billing Account Administrators and Billing Account Users IAM roles for the target account.
