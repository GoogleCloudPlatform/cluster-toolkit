
resource "random_pet" "vpc_name" {
  length    = 2
  separator = "-"
  keepers   = {}
}

locals {
  vpc_key = random_pet.vpc_name.id
}

# VPC Creation
resource "google_compute_network" "network" {
  name                    = "${local.vpc_key}-network"
  project                 = var.project
  auto_create_subnetworks = false
}


resource "google_compute_router" "network_router" {
  name    = "${local.vpc_key}-compute-router"
  region  = var.region
  project = var.project
  network = google_compute_network.network.name
}

resource "google_compute_router_nat" "network_nat" {
  name                               = "${local.vpc_key}-nat"
  project                            = var.project
  router                             = google_compute_router.network_router.name
  region                             = google_compute_router.network_router.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}



# Firewalls - allow all internal, allow ssh from external

resource "google_compute_firewall" "firewall_allow_ssh" {
  name          = "${local.vpc_key}-firewall-ssh"
  network       = google_compute_network.network.name
  project       = var.project
  source_ranges = ["0.0.0.0/0"]
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}


resource "google_compute_firewall" "firewall_internal" {
  name          = "${local.vpc_key}-firewall-internal"
  network       = google_compute_network.network.name
  project       = var.project
  source_ranges = ["10.0.0.0/8"]
  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }
  allow {
    protocol = "udp"
    ports    = ["0-65535"]
  }
  allow { protocol = "icmp" }
}

output "vpc_id" {
  value       = google_compute_network.network.name
  description = "Name of the created VPC"
}
