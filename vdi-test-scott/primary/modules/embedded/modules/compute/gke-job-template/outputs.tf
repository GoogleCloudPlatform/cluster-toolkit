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

output "instructions" {
  description = "Instructions for submitting the GKE job."
  value       = <<-EOT
    A GKE job file has been created locally at:
      ${abspath(local.job_template_output_path)}
    
    Use the following commands to:
    Submit your job:
      kubectl create -f ${abspath(local.job_template_output_path)}
  EOT
}
