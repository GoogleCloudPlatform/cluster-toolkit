resource "google_compute_network" "hosting_vpc" {

  project                 = var.project_id
  name                    = "${var.deployment_name}-network"
  auto_create_subnetworks = false

}


resource "google_compute_subnetwork" "hosting_subnetwork" {

  name          = "${var.deployment_name}-subnetwork"
  ip_cidr_range = "10.2.0.0/28"
  region        = var.region
  network       = google_compute_network.hosting_vpc.name

}


resource "google_compute_firewall" "allow_http_rule" {

  project = var.project_id
  name    = "${var.deployment_name}-allow-http"
  network = google_compute_network.hosting_vpc.name

  allow {
    protocol = "tcp"
    ports    = ["80"]
  }

  source_tags   = ["http-server"]
  source_ranges = ["0.0.0.0/0"]

}


resource "google_compute_firewall" "allow_https_rule" {

  project = var.project_id
  name    = "${var.deployment_name}-allow-https"
  network = google_compute_network.hosting_vpc.name

  allow {
    protocol = "tcp"
    ports    = ["443"]
  }

  source_tags   = ["https-server"]
  source_ranges = ["0.0.0.0/0"]

}

resource "google_compute_firewall" "allow_ssh_rule" {

  project = var.project_id
  name    = "${var.deployment_name}-allow-ssh"
  network = google_compute_network.hosting_vpc.name

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_tags   = ["ssh-server"]
  source_ranges = ["0.0.0.0/0"]

}

