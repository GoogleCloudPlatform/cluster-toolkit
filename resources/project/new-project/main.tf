module "project_factory" {
  source  = "terraform-google-modules/project-factory/google"
  version = "~> 10.1"

  name                    = var.project_id
  random_project_id       = true
  folder_id               = var.folder_id
  org_id                  = var.org_id
  billing_account         = var.billing_account
  default_service_account = var.default_service_account
}
