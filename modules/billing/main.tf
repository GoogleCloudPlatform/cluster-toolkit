resource "google_billing_budget" "budget" {
  count = var.module_enabled ? 1 : 0

  billing_account = var.billing_account
  display_name    = "budget-hpc-${var.project_id}"

  budget_filter {
    projects = ["projects/${var.project_id}"]
    credit_types_treatment = "EXCLUDE_ALL_CREDITS"
    custom_period { 
      start_date {
        year = var.budget_start_date_year
        month = var.budget_start_date_month
        day = var.budget_start_date_day
      }
      end_date {
        year = var.budget_end_date_year
        month = var.budget_end_date_month
        day = var.budget_end_date_day
      }
    }
  }
  amount {
    specified_amount {
        currency_code = var.currency_code
        units         = var.amount
    }

    #NOTE: according to the docs, this needs to be set to null if unsed.
    # For details please see https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/billing_budget#last_period_amount
    last_period_amount =  null
  }

  dynamic "threshold_rules" {
    for_each = var.threshold_rules

    content {
      threshold_percent = threshold_rules.value
      spend_basis       = try(threshold_rules.value.spend_basis, null)
    }
  }

  all_updates_rule {
    monitoring_notification_channels = [
      google_monitoring_notification_channel.scientist_notification_channel.id,
      google_monitoring_notification_channel.manager_notification_channel.id
    ]
    disable_default_iam_recipients = true
  }

  depends_on = [var.module_depends_on]
}


resource "google_monitoring_notification_channel" "scientist_notification_channel" {
  display_name = "Budget Notification Channel for scientist"
  type         = "email"
  project      = var.project_id

  labels = {
    email_address = var.owner
  }
}

resource "google_monitoring_notification_channel" "manager_notification_channel" {
  display_name = "Budget Notification Channel for manager"
  type         = "email"
  project      = var.project_id

  labels = {
    email_address = var.manager
  }
}
