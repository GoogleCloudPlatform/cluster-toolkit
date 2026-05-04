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

resource "null_resource" "run_migrations" {
  triggers = {
    migrations_hash = sha256(join("", [for f in fileset(var.migrations_dir, "${var.sub_directory}/*.up.sql") : filesha256("${var.migrations_dir}/${f}")]))
    proto_hash      = var.proto_descriptors_file != null ? filesha256(var.proto_descriptors_file) : ""
    instance_name   = var.instance_name
    database_name   = var.database_name
  }

  provisioner "local-exec" {
    command = <<EOF
      for f in "${var.migrations_dir}/${var.sub_directory}"/*.up.sql; do
        [ -e "$f" ] || continue
        echo "Applying $f..."
        gcloud spanner databases ddl update "${var.database_name}" \
          --instance="${var.instance_name}" \
          --project="${var.project_id}" \
          ${var.proto_descriptors_file != null ? "--proto-descriptors-file=\"${var.proto_descriptors_file}\" \\" : ""}
          --ddl-file="$f"
      done
    EOF
  }
}
