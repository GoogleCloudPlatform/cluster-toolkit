/**
 * Copyright 2022 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

locals {
  server_args = join(" ", [
    "-e host_type=${var.pbs_role}",
    "-e rpm_url=${var.rpm_url}",
    "-e pbs_license_info=\"${var.pbs_license_server_port}@${var.pbs_license_server}\"",
    "-e pbs_data_service_user=${var.pbs_data_service_user}",
    "-e pbs_exec=${var.pbs_exec}",
    "-e pbs_home=${var.pbs_home}",
    "-e pbs_server=${var.pbs_server}",
  ])

  client_args = join(" ", [
    "-e host_type=${var.pbs_role}",
    "-e rpm_url=${var.rpm_url}",
    "-e pbs_data_service_user=${var.pbs_data_service_user}",
    "-e pbs_exec=${var.pbs_exec}",
    "-e pbs_home=${var.pbs_home}",
    "-e pbs_server=${var.pbs_server}",
  ])

  execution_args = local.client_args

  args_map = {
    "server"    = local.server_args
    "client"    = local.client_args
    "execution" = local.execution_args
  }

  runner = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/scripts/pbs_install.yml")
    "destination" = "pbs_install.yml"
    "args"        = local.args_map[var.pbs_role]
  }
}
