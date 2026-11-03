# Copyright 2026 "Google LLC"
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

output "certificate_map" {
  description = "ID of the certificate map, as a load balancer's target HTTPS proxy takes it."
  # Built from its name rather than read from id, so it is known at plan time:
  # a load balancer decides from it whether to create an HTTPS proxy at all.
  # A proxy may use the map while its entries are still being created.
  value = "projects/${var.project_id}/locations/global/certificateMaps/${google_certificate_manager_certificate_map.map.name}"
}

output "certificate_id" {
  description = "ID of the certificate."
  value       = google_certificate_manager_certificate.certificate.id
}

output "dns_records" {
  description = <<-EOT
    The CNAME records that authorise the certificate, as {name, type, data}.
    Create them in the domains' DNS zones; the certificate is issued once they
    resolve. They change if the authorizations are recreated.
    EOT
  value = [
    for authorization in google_certificate_manager_dns_authorization.domains : {
      name = authorization.dns_resource_record[0].name
      type = authorization.dns_resource_record[0].type
      data = authorization.dns_resource_record[0].data
    }
  ]
}
