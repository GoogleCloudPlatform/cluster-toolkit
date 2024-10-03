/**
 * Copyright 2023 Google LLC
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
  setx_http_proxy_ps1 = !var.http_proxy_set_environment ? [] : [
    templatefile("${path.module}/templates/setx_http_proxy.ps1", {
      "http_proxy" : var.http_proxy,
      "no_proxy" : var.no_proxy,
    })
  ]

  nvidia_ps1 = !var.install_nvidia_driver ? [] : [
    templatefile("${path.module}/templates/install_gpu_driver.ps1.tftpl", {
      "url" : var.install_nvidia_driver_script
      "args" : var.install_nvidia_driver_args
      "http_proxy" : var.http_proxy,
    })
  ]

  startup_ps1 = concat(local.setx_http_proxy_ps1, local.nvidia_ps1)
}
