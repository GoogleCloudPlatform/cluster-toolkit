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

check "disk_type_c4_compatibility" {
  assert {
    condition     = !(can(regex("^c4-", var.machine_type)) && var.disk_type == "pd-ssd")
    error_message = "The C4 machine series does not support pd-ssd. Please use hyperdisk-balanced or another compatible disk type."
  }
}


check "disk_type_c2_compatibility" {
  assert {
    condition     = !(can(regex("^c2-", var.machine_type)) && can(regex("hyperdisk", var.disk_type)))
    error_message = "The C2 machine series does not support Hyperdisk as a boot disk. Please use a compatible disk type like pd-ssd, pd-standard, or pd-balanced."
  }
}


check "disk_type_pd_extreme_compatibility" {
  assert {
    condition     = var.disk_type != "pd-extreme" || can(regex("^(m1-|m2-|m3-|n2-|n2d-)", var.machine_type))
    error_message = "pd-extreme disks are only supported for M1, M2, M3, N2, and N2D machine series."
  }
}


check "disk_type_hyperdisk_extreme_compatibility" {
  assert {
    condition     = var.disk_type != "hyperdisk-extreme" || can(regex("^(c3-|m1-|m3-|n2-)", var.machine_type))
    error_message = "hyperdisk-extreme disks are only supported for C3, M1, M3, and N2 machine series."
  }
}


check "disk_type_hyperdisk_throughput_compatibility" {
  assert {
    condition     = var.disk_type != "hyperdisk-throughput" || can(regex("^(c3-|c3d-|n4-|n2-|n2d-|n1-|t2d-|m1-)", var.machine_type))
    error_message = "hyperdisk-throughput disks are only supported for C3, C3D, N4, N2, N2D, N1, T2D, and M1 machine series."
  }
}
