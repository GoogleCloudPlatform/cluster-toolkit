variable "project_id" {
  description = "project ID"
  type = string
}

variable "budget_start_date_year" {
  description = "Year of the date to start budget with. Must be from 1 to 9999"
  type = number
}
variable "budget_start_date_month" {
  description = "Month of a year to start budget with. Must be from 1 to 12"
  type = number
}
variable "budget_start_date_day" {
  description = "Day of a month to start budget with. Must be from 1 to 31 and valid for the year and month"
  type = number
}
variable "budget_end_date_year" {
  description = "Year of the date to end budget with. Must be from 1 to 9999"
  type = number
}
variable "budget_end_date_month" {
  description = "Month of a year to end budget with. Must be from 1 to 12"
  type = number
}
variable "budget_end_date_day" {
  description = "Day of a month to end budget with. Must be from 1 to 31 and valid for the year and month"
  type = number
}

variable "owner" {
  description = "Owner of the project"
  type = string
}

variable "manager" {
  description = "Manager who approved the project"
  type = string
}

variable "billing_account" {
  type        = string
  description = "(Required) ID of the billing account to set a budget on."
}

variable "threshold_rules" {
  type = list(number)
  description = "(Required) Rules that trigger alerts (notifications of thresholds being crossed) when spend exceeds the specified percentages of the budget."
}

variable "amount" {
  type        = number
  description = "A specified amount to use as the budget."
}

variable "currency_code" {
  type        = string
  description = "The 3-letter currency code defined in ISO 4217. If specified, it must match the currency of the billing account. For a list of currency codes, please see https://en.wikipedia.org/wiki/ISO_4217"
}


# ----------------------------------------------------------------------------------------------------------------------
# MODULE CONFIGURATION PARAMETERS
# These variables are used to configure the module.
# ----------------------------------------------------------------------------------------------------------------------

variable "module_enabled" {
  type        = bool
  description = "(Optional) Whether to create resources within the module or not."
  default     = true
}

variable "module_timeouts" {
  description = "(Optional) How long certain operations (per resource type) are allowed to take before being considered to have failed."
  type        = any
  default = {}
}

variable "module_depends_on" {
  type        = any
  description = "(Optional) A list of external resources the module depends on."
  default     = []
}
