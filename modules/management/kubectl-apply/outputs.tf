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

output "ready" {
  description = "True if the module is ready (manifests applied and charts installed)"
  value       = true
  depends_on = [
    module.install_kueue,
    module.configure_kueue,
    module.install_jobset,
    module.install_gpu_operator,
    module.install_nvidia_dra_driver,
    module.install_gib,
    module.install_asapd_lite,
    module.kubectl_apply_manifests,
  ]
}
