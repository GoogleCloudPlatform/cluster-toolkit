# Copyright 2026 Google LLC
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

resource "null_resource" "apply_kueue_locks" {
  triggers = {
    dummy_hash  = filemd5("${path.module}/../daily-tests/blueprints/test-infra-kueue/configs/dummy-device-plugin.yaml")
    setup_hash  = filemd5("${path.module}/../daily-tests/blueprints/test-infra-kueue/configs/kueue-setup.yaml")
    script_hash = filemd5("${path.module}/apply_kueue_locks.sh")
  }

  provisioner "local-exec" {
    command = "${path.module}/apply_kueue_locks.sh"
  }
}
