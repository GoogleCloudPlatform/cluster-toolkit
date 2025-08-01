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

resource "terraform_data" "input_validation" {
  lifecycle {
    precondition {
      condition     = contains(["tigervnc"], var.vnc_flavor)
      error_message = "vnc_flavor must be tigervnc."
    }

    precondition {
      condition     = contains(["guacamole"], var.vdi_tool)
      error_message = "vdi_tool must be one of: guacamole."
    }

    precondition {
      condition     = contains(["local_users"], var.user_provision)
      error_message = "user_provision must be local_users."
    }

    precondition {
      condition     = can(regex("^[1-9][0-9]*x[1-9][0-9]*$", var.vdi_resolution))
      error_message = "vdi_resolution must be in the form WIDTHxHEIGHT (e.g. 1920x1080)."
    }

    precondition {
      condition     = var.user_provision == "local_users" || length(var.vdi_users) == 0
      error_message = "vdi_users may only be set when user_provision = local_users."
    }

    precondition {
      condition = (
        var.vnc_flavor != "tigervnc" && var.vnc_flavor != "tightvnc"
        ) || alltrue([
          for user in var.vdi_users : (
            user.port >= var.vnc_port_min && user.port <= var.vnc_port_max
          )
      ])
      error_message = "Each VDI user must have a port between 5901 and 5999 when VNC is used."
    }

    precondition {
      condition = alltrue([
        for user in var.vdi_users : (
          user.reset_password == null || user.reset_password == true || user.reset_password == false
        )
      ])
      error_message = "reset_password must be a boolean value (true/false) or null."
    }
  }
}
