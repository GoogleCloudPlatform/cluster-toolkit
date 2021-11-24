module "bucket" {
  source  = "terraform-google-modules/cloud-storage/google//modules/simple_bucket"
  version = "~> 1.3"

  name       = "${var.project_id}-bucket"
  project_id = var.project_id
  location   = var.region
}

