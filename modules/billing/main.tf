resource "google_billing_budget" "budget" {
  count = var.module_enabled ? 1 : 0

  billing_account = var.billing_account
  display_name    = var.display_name

  dynamic "budget_filter" {
    for_each = var.budget_filter != null ? [var.budget_filter] : []

    content {
      projects               = try(toset(budget_filter.value.projects), null)
      credit_types_treatment = try(budget_filter.value.credit_types_treatment, "INCLUDE_ALL_CREDITS")
      services               = try(toset(budget_filter.value.services), null)
      credit_types           = try(toset(budget_filter.value.credit_types), null)
      subaccounts            = try(toset(budget_filter.value.subaccounts), null)
      labels                 = try(budget_filter.value.labels, null)
    }
  }

  amount {
    dynamic "specified_amount" {
      for_each = !var.use_last_period_amount ? [true] : []

      content {
        currency_code = var.currency_code
        units         = var.amount
      }
    }

    #NOTE: according to the docs, this needs to be set to null if unsed.
    # For details please see https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/billing_budget#last_period_amount
    last_period_amount = var.use_last_period_amount ? true : null
  }

  dynamic "threshold_rules" {
    for_each = var.threshold_rules

    content {
      threshold_percent = threshold_rules.value
      spend_basis       = try(threshold_rules.value.spend_basis, null)
    }
  }

  dynamic "all_updates_rule" {
    for_each = var.notifications != null ? [var.notifications] : []

    content {
      pubsub_topic                     = try(all_updates_rule.value.pubsub_topic, null)
      schema_version                   = try(all_updates_rule.value.pubsub_topic, null)
      monitoring_notification_channels = try(all_updates_rule.value.monitoring_notification_channels, null)
      disable_default_iam_recipients   = try(all_updates_rule.value.disable_default_iam_recipients, false)
    }
  }

  dynamic "timeouts" {
    for_each = try([var.module_timeouts.google_billing_budget], [])

    content {
      create = try(timeouts.value.create, null)
      update = try(timeouts.value.update, null)
      delete = try(timeouts.value.delete, null)
    }
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
