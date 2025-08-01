# Copyright 2025 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "vdi-setup", ghpc_role = "scripts" })
}

# Create a tar.gz of roles/ directory
data "archive_file" "roles_tar" {
  type        = "tar.gz"
  source_dir  = "${path.module}/roles"
  output_path = "${path.module}/roles.tar.gz"
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

# Generate vars content using templatefile
locals {
  vdi_vars_content = templatefile("${path.module}/templates/vars.yaml.tftpl", {
    deployment_name             = var.deployment_name
    project_id                  = var.project_id
    user_provision              = var.user_provision
    vnc_flavor                  = var.vnc_flavor
    vdi_tool                    = var.vdi_tool
    vdi_user_group              = var.vdi_user_group
    vdi_webapp_port             = var.vdi_webapp_port
    vdi_resolution              = var.vdi_resolution
    vdi_users                   = var.vdi_users
    debug                       = var.debug
    reset_webapp_admin_password = var.reset_webapp_admin_password
    force_rerun                 = var.force_rerun
    vdi_bucket_name             = local.bucket_name
    zone                        = var.zone
  })
}

# Assemble runners
locals {
  runners = [
    # Install dependencies
    {
      type        = "shell"
      destination = "install-deps.sh"
      content     = <<-EOT
        #!/bin/bash
        set -eux
        /usr/local/ghpc-venv/bin/python3 -m pip install requests google-auth docker
        ansible-galaxy collection install google.cloud
      EOT
    },
    # Stage roles.tar.gz to temporary location
    {
      type        = "data"
      source      = data.archive_file.roles_tar.output_path
      destination = "/tmp/vdi/roles.tar.gz"
    },

    # Unpack into /opt/vdi-setup/roles (final location)
    {
      type        = "shell"
      destination = "unpack_roles.sh"
      content     = <<-EOT
        #!/bin/bash
        set -eux
        mkdir -p /opt/vdi-setup/roles
        tar xzf /tmp/vdi/roles.tar.gz -C /opt/vdi-setup/roles
        # Clean up temporary file
        rm -f /tmp/vdi/roles.tar.gz
      EOT
    },

    # write out vars file as YAML to final location
    {
      type        = "data"
      content     = local.vdi_vars_content
      destination = "/opt/vdi-setup/vars.yaml"
    },

    # Run the rendered playbook via ansible-local from final location
    {
      type = "ansible-local"
      content = templatefile("${path.module}/templates/install.yaml.tftpl",
        {
          roles = ["lock_manager", "base_os", "secret_manager", "user_provision", "vnc", "vdi_tool", "vdi_monitor"],
        }
      )
      destination = "/opt/vdi-setup/install.yaml"
      args = var.debug ? "--extra-vars @/opt/vdi-setup/vars.yaml -v --extra-vars debug=true" : "--extra-vars @/opt/vdi-setup/vars.yaml"
    },
    # Clean up temporary directory
    {
      type        = "shell"
      destination = "cleanup.sh"
      content     = <<-EOT
        #!/bin/bash
        set -eux
        rm -rf /tmp/vdi
      EOT
    },
  ]

  bucket_name = "${substr(var.deployment_name, 0, 39)}-vdi-scripts-${random_id.resource_name_suffix.hex}"
}

# Bucket to stage runners
resource "google_storage_bucket" "bucket" {
  labels                      = local.labels
  project                     = var.project_id
  name                        = local.bucket_name
  location                    = var.region
  uniform_bucket_level_access = true
  storage_class               = "REGIONAL"
}

# Use the startup-script module to push and execute them
module "startup_script" {
  labels          = local.labels
  source          = "../../../../modules/scripts/startup-script"
  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region

  runners         = local.runners
  gcs_bucket_path = "gs://${google_storage_bucket.bucket.name}"

  docker = {
    enabled = true
  }
}

# Expose the combined startup script
locals {
  combined_runner = {
    type        = "shell"
    content     = module.startup_script.startup_script
    destination = "install-vdi-and-setup.sh"
  }
}
