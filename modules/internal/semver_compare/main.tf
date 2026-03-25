locals {
  # Strip leading 'v' if present
  clean_version     = trimprefix(var.current_version, "v")
  clean_min_version = trimprefix(var.minimum_version, "v")

  # Regex to capture strictly major.minor.patch integers, and optionally a -gke.123 build suffix
  version_regex = "^([0-9]+)\\.([0-9]+)\\.?([0-9]*)(?:-gke\\.([0-9]+))?"

  # Try to parse. If it fails (e.g., 'sha256-12345'), it returns null.
  parsed_ver = try(regex(local.version_regex, local.clean_version), null)
  parsed_min = try(regex(local.version_regex, local.clean_min_version), null)

  is_valid_semver = local.parsed_ver != null && local.parsed_min != null

  # Map to integers, defaulting to 0 for missing patch versions or gke build numbers
  ver_major = local.is_valid_semver ? tonumber(local.parsed_ver[0]) : 0
  ver_minor = local.is_valid_semver ? tonumber(local.parsed_ver[1]) : 0
  ver_patch = local.is_valid_semver ? (try(local.parsed_ver[2], "") != "" ? tonumber(local.parsed_ver[2]) : 0) : 0
  ver_gke   = local.is_valid_semver ? (try(local.parsed_ver[3], null) != null && try(local.parsed_ver[3], "") != "" ? tonumber(local.parsed_ver[3]) : 0) : 0

  min_major = local.is_valid_semver ? tonumber(local.parsed_min[0]) : 0
  min_minor = local.is_valid_semver ? tonumber(local.parsed_min[1]) : 0
  min_patch = local.is_valid_semver ? (try(local.parsed_min[2], "") != "" ? tonumber(local.parsed_min[2]) : 0) : 0
  min_gke   = local.is_valid_semver ? (try(local.parsed_min[3], null) != null && try(local.parsed_min[3], "") != "" ? tonumber(local.parsed_min[3]) : 0) : 0

  # Fail-open logic for custom tags
  is_greater_than_or_equal = (
    !local.is_valid_semver ||
    local.ver_major > local.min_major ||
    (local.ver_major == local.min_major && local.ver_minor > local.min_minor) ||
    (local.ver_major == local.min_major && local.ver_minor == local.min_minor && local.ver_patch > local.min_patch) ||
    (local.ver_major == local.min_major && local.ver_minor == local.min_minor && local.ver_patch == local.min_patch && local.ver_gke >= local.min_gke)
  )
}