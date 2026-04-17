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

output "instructions_a4-cluster" {
  description = "Generated output from module 'a4-cluster'"
  value       = module.a4-cluster.instructions
}

output "instructions_a4-pool" {
  description = "Generated output from module 'a4-pool'"
  value       = module.a4-pool.instructions
}

output "instructions_job-template" {
  description = "Generated output from module 'job-template'"
  value       = module.job-template.instructions
}

output "instructions_fio-bench-job-template" {
  description = "Generated output from module 'fio-bench-job-template'"
  value       = module.fio-bench-job-template.instructions
}
