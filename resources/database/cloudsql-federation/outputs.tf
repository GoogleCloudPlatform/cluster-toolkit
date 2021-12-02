output "cloudsql" {
  description = "Describes a cloudsql instance."
  value = {
    server_ip = google_sql_database_instance.instance.ip_address[0].ip_address
    user      = google_sql_user.users.name
    password  = google_sql_user.users.password
    db_name   = google_sql_database.database.name
  }
}
