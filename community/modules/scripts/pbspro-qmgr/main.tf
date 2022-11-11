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
  server_conf_file = "/var/tmp/pbs_server_conf"
  args = join(" ", [
    "-e client_host_count=${var.client_host_count}",
    "-e client_hostname_prefix=${var.client_hostname_prefix}",
    "-e execution_host_count=${var.execution_host_count}",
    "-e execution_hostname_prefix=${var.execution_hostname_prefix}",
    "-e server_conf_file=${local.server_conf_file}",
  ])

  runners = [
    {
      "type"        = "data"
      "content"     = var.server_conf
      "destination" = local.server_conf_file
    },
    {
      "type"        = "ansible-local"
      "content"     = file("${path.module}/scripts/pbs_qmgr.yml")
      "destination" = "pbs_qmgr.yml"
      "args"        = local.args
    },
  ]
}
