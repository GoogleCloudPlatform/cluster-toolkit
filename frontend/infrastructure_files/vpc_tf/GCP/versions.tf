terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 3.54"
    }
    random = {
      version = ">= 3.0"
    }
  }

  required_version = ">= 0.12.20"
}


provider "google" {
  project = var.project
  region  = var.region
}

