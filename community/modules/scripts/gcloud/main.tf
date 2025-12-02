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

locals {
  create_script_content = <<EOT
#!/bin/bash
set -e -x
echo "Executing gcloud CREATE commands for ${var.module_instance_id}..."
%{for cmd_pair in var.commands~}
${cmd_pair.create}
%{endfor~}
echo "All gcloud CREATE commands executed successfully for ${var.module_instance_id}."
EOT

  destroy_script_content = <<EOT
#!/bin/bash
set -x # We don't use -e here to allow other cleanup commands to run if one fails
echo "Executing gcloud DELETE commands for ${var.module_instance_id}..."
%{for cmd_pair in reverse(var.commands)~}
${cmd_pair.delete} || echo "Delete command failed: ${cmd_pair.delete}, continuing..."
%{endfor~}
echo "All gcloud DELETE commands attempted for ${var.module_instance_id}."
EOT
}

resource "null_resource" "gcloud_commands" {
  triggers = {
    commands_hash          = sha256(jsonencode(var.commands))
    module_instance_id     = var.module_instance_id
    create_script_content  = local.create_script_content
    destroy_script_content = local.destroy_script_content
  }

  provisioner "local-exec" {
    command = "/bin/bash <<EOT\n${self.triggers.create_script_content}\nEOT"
  }

  provisioner "local-exec" {
    when    = destroy
    command = "/bin/bash <<EOT\n${self.triggers.destroy_script_content}\nEOT"
  }
}
