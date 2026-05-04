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

resource "null_resource" "helm_upgrade" {
  triggers = {
    values = yamlencode(var.set_values)
  }
  provisioner "local-exec" {
    command = <<EOT
      gcloud container clusters get-credentials ${var.cluster_name} --project ${var.project_id} --region ${var.location}
      helm upgrade --install ${var.release_name} ${var.chart_name} \
        --namespace ${var.namespace} --create-namespace \
        ${join(" ", [for f in var.values_yaml : "--values ${f}"])} \
        ${join(" ", [for v in var.set_values : "--set '${v.name}=${v.value}'"])}
    EOT
  }
}
