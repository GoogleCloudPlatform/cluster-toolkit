
locals {
  dash_path = "${path.module}/dashboards/${var.base_dashboard}.json.tpl"
}

resource "google_monitoring_dashboard" "dashboard" {
  dashboard_json = templatefile(local.dash_path, {
    widgets         = var.widgets
    deployment_name = var.deployment_name
    }
  )
  project = var.project_id
}
