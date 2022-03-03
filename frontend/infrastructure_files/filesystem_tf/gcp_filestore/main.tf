resource "random_pet" "fs_name" {
    length = 2
    separator = "-"
    keepers = { }
}

locals {
    fs_key = random_pet.fs_name.id
}

resource "google_filestore_instance" "instance" {
    name = local.fs_key
    location = var.zone
    tier = var.tier

    file_shares {
        capacity_gb = 2660
        name        = var.export_name
    }

    networks {
        network = var.network_id
        modes   = ["MODE_IPV4"]
    }
}

output "fs_id" {
    value = google_filestore_instance.instance.name
}
output "hostname" {
    value = google_filestore_instance.instance.networks[0].ip_addresses[0]
}
