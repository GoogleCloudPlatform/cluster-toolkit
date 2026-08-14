/**
* Copyright 2026 Google LLC
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

locals {
  target_dir = var.sub_directory != "" ? "${var.migrations_dir}/${var.sub_directory}" : var.migrations_dir
}

resource "null_resource" "run_migrations" {
  triggers = {
    migrations_hash = sha256(join("", [for f in fileset(local.target_dir, "*.up.sql") : filesha256("${local.target_dir}/${f}")]))

    proto_hash    = var.proto_descriptors_file != null ? filesha256(var.proto_descriptors_file) : ""
    instance_name = var.instance_name
    database_name = var.database_name
    project_id    = var.project_id
  }

  provisioner "local-exec" {
    command = <<EOF
      set -e
      for f in "${local.target_dir}"/*.up.sql; do
        [ -e "$f" ] || continue
        echo "Applying $f..."
        gcloud spanner databases ddl update "${var.database_name}" \
          --instance="${var.instance_name}" \
          --project="${var.project_id}" \
          --ddl-file="$f" \
          ${var.proto_descriptors_file != null ? "--proto-descriptors-file=\"${var.proto_descriptors_file}\"" : ""} || exit 1
      done
    EOF
  }
}
