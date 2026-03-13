# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "artifact-registry", ghpc_role = "container" })
}

locals {
  # Auto (i.e., empty) vs user-managed replication
  auto = length(var.user_managed_replication) == 0 ? true : false

  # For remote custom repositories, parse out host to create a base_component name
  mirror_url_no_proto = var.repo_mirror_url != null ? replace(replace(var.repo_mirror_url, "https://", ""), "http://", "") : ""
  mirror_host         = local.mirror_url_no_proto != "" ? split("/", local.mirror_url_no_proto)[0] : ""

  base_component = replace(
    replace(
      replace(
        lower(
          local.mirror_host != ""
          ? "${var.format}-${var.repo_mode}-${local.mirror_host}"
          : "${var.format}-${var.repo_mode}-nohost"
        ),
        "\\.", "-"
      ),
      "/", "-"
    ),
    "_", "-"
  )

  repository_suffix = random_id.resource_name_suffix.hex

  # The final name for the artifact registry repository
  repository_name = replace(
    replace(
      lower(
        format("%s-%s", local.base_component, local.repository_suffix)
      ),
      ".", "-"
    ),
    "/", "-"
  )

  # The secret name is derived from the repository name
  # with a suffix like "-secret".
  derived_secret_name = format("%s-secret", local.repository_name)
}

##############################
# PASSWORD / SECRET 
##############################

# Only create a random password if user didn't supply one
resource "random_password" "repo_password" {
  count            = var.use_upstream_credentials && var.repo_password == null ? 1 : 0
  length           = 24
  special          = true
  override_special = "_-#=."
}

resource "google_secret_manager_secret" "repo_password_secret" {
  count   = var.use_upstream_credentials ? 1 : 0
  project = var.project_id

  # Derive the secret ID from the repository name
  secret_id = local.derived_secret_name

  labels = local.labels

  replication {
    dynamic "auto" {
      for_each = local.auto ? [1] : []
      content {}
    }
    dynamic "user_managed" {
      for_each = local.auto ? [] : [1]
      content {
        dynamic "replicas" {
          for_each = var.user_managed_replication
          content {
            location = replicas.value.location
            dynamic "customer_managed_encryption" {
              for_each = replicas.value.kms_key_name != null ? [1] : []
              content {
                kms_key_name = customer_managed_encryption.value
              }
            }
          }
        }
      }
    }
  }
}

resource "google_secret_manager_secret_version" "repo_password_secret_version" {
  count  = var.use_upstream_credentials ? 1 : 0
  secret = google_secret_manager_secret.repo_password_secret[0].id

  # If user provided a password, use it. Otherwise use the random password.
  secret_data = var.repo_password != null ? var.repo_password : random_password.repo_password[0].result
}

##############################
# IAM BINDINGS
##############################

##############################
# ARTIFACT REGISTRY
##############################

resource "random_id" "resource_name_suffix" {
  byte_length = 2
}

resource "google_artifact_registry_repository" "artifact_registry" {
  project       = var.project_id
  location      = var.region
  format        = var.format
  mode          = var.repo_mode
  description   = var.deployment_name
  labels        = local.labels
  repository_id = local.repository_name

  # Only create remote_repository_config if REMOTE_REPOSITORY
  dynamic "remote_repository_config" {
    for_each = var.repo_mode == "REMOTE_REPOSITORY" ? [1] : []
    content {
      description = "Pull-through cache"

      dynamic "docker_repository" {
        for_each = var.format == "DOCKER" && var.repo_public_repository != null ? [1] : []
        content {
          public_repository = var.repo_public_repository
        }
      }

      dynamic "docker_repository" {
        for_each = var.format == "DOCKER" && var.repo_public_repository == null && var.repo_mirror_url != null ? [1] : []
        content {
          custom_repository {
            uri = var.repo_mirror_url
          }
        }
      }

      dynamic "maven_repository" {
        for_each = var.format == "MAVEN" && var.repo_public_repository != null ? [1] : []
        content {
          public_repository = var.repo_public_repository
        }
      }

      dynamic "maven_repository" {
        for_each = var.format == "MAVEN" && var.repo_public_repository == null && var.repo_mirror_url != null ? [1] : []
        content {
          custom_repository {
            uri = var.repo_mirror_url
          }
        }
      }

      dynamic "npm_repository" {
        for_each = var.format == "NPM" && var.repo_public_repository != null ? [1] : []
        content {
          public_repository = var.repo_public_repository
        }
      }

      dynamic "npm_repository" {
        for_each = var.format == "NPM" && var.repo_public_repository == null && var.repo_mirror_url != null ? [1] : []
        content {
          custom_repository {
            uri = var.repo_mirror_url
          }
        }
      }

      dynamic "python_repository" {
        for_each = var.format == "PYTHON" && var.repo_public_repository != null ? [1] : []
        content {
          public_repository = var.repo_public_repository
        }
      }

      dynamic "python_repository" {
        for_each = var.format == "PYTHON" && var.repo_public_repository == null && var.repo_mirror_url != null ? [1] : []
        content {
          custom_repository {
            uri = var.repo_mirror_url
          }
        }
      }

      dynamic "apt_repository" {
        for_each = var.format == "APT" && var.repo_public_repository != null ? [1] : []
        content {
          public_repository {
            repository_base = var.repository_base
            repository_path = var.repository_path
          }
        }
      }

      dynamic "apt_repository" {
        for_each = var.format == "APT" && var.repo_public_repository == null && var.repo_mirror_url != null ? [1] : []
        content {
          custom_repository {
            uri = var.repo_mirror_url
          }
        }
      }

      dynamic "yum_repository" {
        for_each = var.format == "YUM" && var.repo_public_repository != null ? [1] : []
        content {
          public_repository {
            repository_base = var.repository_base
            repository_path = var.repository_path
          }
        }
      }

      dynamic "yum_repository" {
        for_each = var.format == "YUM" && var.repo_public_repository == null && var.repo_mirror_url != null ? [1] : []
        content {
          custom_repository {
            uri = var.repo_mirror_url
          }
        }
      }

      dynamic "common_repository" {
        for_each = var.format == "COMMON" ? [1] : []
        content {
          uri = var.repo_mirror_url
        }
      }

      # Only enable upstream credentials if user wants it
      dynamic "upstream_credentials" {
        for_each = var.use_upstream_credentials ? [1] : []
        content {
          username_password_credentials {
            username                = var.repo_username
            password_secret_version = google_secret_manager_secret_version.repo_password_secret_version[0].name
          }
        }
      }
    }
  }

  depends_on = [
    google_secret_manager_secret.repo_password_secret,
    google_secret_manager_secret_version.repo_password_secret_version,
  ]
}
