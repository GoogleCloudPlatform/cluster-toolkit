output "is_valid_semver" {
  value       = local.is_valid_semver
  description = "True if both versions could be parsed into major.minor semantic logic."
}

output "is_greater_than_or_equal" {
  value       = local.is_greater_than_or_equal
  description = "True if the version meets the minimum requirement, or if the version is a non-standard custom string (fail-open)."
}