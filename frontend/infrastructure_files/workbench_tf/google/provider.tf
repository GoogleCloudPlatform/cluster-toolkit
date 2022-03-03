provider "google" {
  credentials = file(var.credentials)
  region      = var.region
  zone        = var.zone
  project     = var.project
}
