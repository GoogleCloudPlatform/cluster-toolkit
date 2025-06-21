# Copyright 2025 "Google LLC"
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

# Data source that runs the permission check script in-line.
data "external" "gcp_permission_check" {
  program = [
    "/bin/bash",
    "-c",
    # The script is updated to output a clean list of missing permissions on failure.
    <<-EOT
      set -e
      if [ "$#" -ne 2 ]; then
        echo "Usage: $0 <project_id> <permissions_comma_separated>" >&2; exit 1
      fi

      PROJECT_ID="$1"
      PERMISSIONS_TO_CHECK="$2"

      TOKEN=$(gcloud auth print-access-token)

      PERMS_JSON_ARRAY=""
      while IFS= read -r p; do
        if [ -z "$PERMS_JSON_ARRAY" ]; then
          PERMS_JSON_ARRAY="\"$p\""
        else
          PERMS_JSON_ARRAY="$PERMS_JSON_ARRAY,\"$p\""
        fi
      done < <(echo "$PERMISSIONS_TO_CHECK" | tr ',' '\n')
      REQUEST_BODY="{\"permissions\":[$PERMS_JSON_ARRAY]}"

      API_RESPONSE=$(curl -s -X POST \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "$REQUEST_BODY" \
        "https://cloudresourcemanager.googleapis.com/v1/projects/$PROJECT_ID:testIamPermissions")

      MISSING_PERMS=""
      while IFS= read -r p; do
        if ! echo "$API_RESPONSE" | grep -q "\"$p\""; then
          if [ -z "$MISSING_PERMS" ]; then
            MISSING_PERMS="$p"
          else
            MISSING_PERMS="$MISSING_PERMS,$p"
          fi
        fi
      done < <(echo "$PERMISSIONS_TO_CHECK" | tr ',' '\n')

      if [ -z "$MISSING_PERMS" ]; then
        echo '{"validation_passed": "true", "missing_permissions_list": ""}'
      else
        # On failure, output the missing permissions in a clean, comma-separated list.
        printf '{"validation_passed": "false", "missing_permissions_list": "%s"}' "$MISSING_PERMS"
      fi
    EOT
    ,
    "permission_checker.sh",
    var.project_id,
    join(",", var.required_permissions)
  ]
}

locals {
  # A map to suggest roles for missing permissions.
  permission_to_role_map = {
    "resourcemanager.projects.setIamPolicy" = "roles/resourcemanager.projectIamAdmin"
    "container.clusters.create"             = "roles/container.admin"
    "container.clusters.delete"             = "roles/container.admin"
    "compute.instances.create"              = "roles/compute.admin"
    "compute.instances.delete"              = "roles/compute.admin"
    "storage.buckets.create"                = "roles/storage.admin"
    "storage.objects.create"                = "roles/storage.admin"
    "iam.serviceAccounts.create"            = "roles/iam.serviceAccountAdmin"
    "iam.serviceAccounts.setIamPolicy"      = "roles/iam.serviceAccountAdmin"
    "iam.serviceAccounts.actAs"             = "roles/iam.serviceAccountUser"
    "serviceusage.services.use"             = "roles/serviceusage.serviceUsageConsumer"
  }

  # Process the script's output and suggest roles.
  validation_passed       = data.external.gcp_permission_check.result.validation_passed == "true"
  missing_permissions_str = lookup(data.external.gcp_permission_check.result, "missing_permissions_list", "")
  missing_permissions     = local.missing_permissions_str == "" ? [] : split(",", local.missing_permissions_str)

  # For each missing permission, look up its suggested role in the map.
  suggested_roles = toset([
    for permission in local.missing_permissions : lookup(local.permission_to_role_map, permission, "No specific role suggestion found for this permission.")
  ])
}

resource "null_resource" "permission_validator" {
  triggers = {
    check_id = join(",", var.required_permissions)
  }

  lifecycle {
    precondition {
      condition     = local.validation_passed
      error_message = <<-EOT

        ----------------------------------------------------------------------
        VALIDATION FAILED: The user is missing required IAM permissions.

        Project ID:          ${var.project_id}
        Missing Permissions: ${jsonencode(local.missing_permissions)}

        Suggested Roles to Acquire: ${jsonencode(tolist(local.suggested_roles))}
        ----------------------------------------------------------------------
      EOT
    }
  }
}
