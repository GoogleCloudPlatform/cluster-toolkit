resource "google_storage_bucket" "test_data_name" {
  name     = var.test_variable
  location = "US"
}
