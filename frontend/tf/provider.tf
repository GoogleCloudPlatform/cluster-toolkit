provider "google" {
  project = var.project_id
  zone    = var.zone
  region  = var.region
}
provider "google-beta" {
  project = var.project_id
  zone    = var.zone
  region  = var.region
}

